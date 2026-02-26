package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fiapx/fiapx-processing-service/internal/infra/config"
	"github.com/fiapx/fiapx-processing-service/internal/infra/email"
	"github.com/fiapx/fiapx-processing-service/internal/infra/ffmpeg"
	"github.com/fiapx/fiapx-processing-service/internal/infra/metrics"
	miniostorage "github.com/fiapx/fiapx-processing-service/internal/infra/minio"
	"github.com/fiapx/fiapx-processing-service/internal/infra/postgres"
	"github.com/fiapx/fiapx-processing-service/internal/infra/rabbitmq"
	"github.com/fiapx/fiapx-processing-service/internal/infra/tracing"
	"github.com/fiapx/fiapx-processing-service/internal/usecase"
	"github.com/fiapx/fiapx-processing-service/pkg/logger"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	fatalOnErr(err, "load config")

	log, err := logger.New(cfg.LogLevel)
	fatalOnErr(err, "init logger")
	defer log.Sync()

	log.Info("starting fiapx-processing-service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Tracing (non-fatal if Jaeger unavailable)
	tp, err := tracing.InitTracer(ctx, cfg.JaegerEndpoint)
	if err != nil {
		log.Warn("tracing init failed, continuing without tracing", zap.Error(err))
	} else {
		defer tp.Shutdown(ctx)
	}

	// Database
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	fatalOnErr(err, "connect to postgres")
	defer pool.Close()

	// Migrations
	err = postgres.RunMigrations(cfg.DatabaseURL, "migrations")
	if err != nil {
		log.Warn("migration warning", zap.Error(err))
	}

	// MinIO
	storage, err := miniostorage.NewStorage(miniostorage.StorageConfig{
		Endpoint:     cfg.MinIOEndpoint,
		AccessKey:    cfg.MinIOAccessKey,
		SecretKey:    cfg.MinIOSecretKey,
		UseSSL:       cfg.MinIOUseSSL,
		UploadBucket: cfg.MinIOUploadBucket,
		ZipBucket:    cfg.MinIOZipBucket,
	})
	fatalOnErr(err, "create minio storage")
	fatalOnErr(storage.EnsureBuckets(ctx), "ensure minio buckets")

	// RabbitMQ publisher connection
	rmqConn, err := amqp.Dial(cfg.RabbitMQURL)
	fatalOnErr(err, "connect to rabbitmq for publisher")
	defer rmqConn.Close()

	pub, err := rabbitmq.NewPublisher(rmqConn, cfg.RabbitMQExchange)
	fatalOnErr(err, "create rabbitmq publisher")

	statusPub := rabbitmq.NewStatusPublisher(pub)
	dlqPub := rabbitmq.NewDLQPublisher(pub, cfg.RabbitMQDLQ)

	// Infra adapters
	repo := postgres.NewJobRepository(pool)
	extractor := ffmpeg.NewExtractor(cfg.FFmpegFPS, cfg.FFmpegFormat, log)
	zipper := ffmpeg.NewZipCreator()
	notifier := email.NewSMTPNotifier(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom, log)

	// Use case
	uc := usecase.NewProcessVideoUseCase(
		repo, storage, extractor, zipper,
		statusPub, dlqPub, notifier,
		log,
		usecase.ProcessVideoConfig{
			TempDir:    cfg.TempDir,
			MaxRetries: cfg.MaxRetries,
		},
	)

	// Metrics server
	metricsSrv := metrics.StartMetricsServer(ctx, cfg.MetricsPort, log)

	// Consumer (worker pool)
	consumer, err := rabbitmq.NewConsumer(rabbitmq.ConsumerConfig{
		URL:         cfg.RabbitMQURL,
		Queue:       cfg.RabbitMQProcessingQueue,
		Exchange:    cfg.RabbitMQExchange,
		DLQ:         cfg.RabbitMQDLQ,
		StatusQueue: cfg.RabbitMQStatusQueue,
		Prefetch:    cfg.RabbitMQPrefetch,
		WorkerCount: cfg.WorkerCount,
		BaseDelayMs: cfg.RetryBaseDelayMs,
	}, uc.Execute, log)
	fatalOnErr(err, "create consumer")

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Info("received shutdown signal", zap.String("signal", sig.String()))
		cancel()
	}()

	log.Info("fiapx-processing-service started, consuming messages")

	if err := consumer.Start(ctx); err != nil {
		log.Error("consumer error", zap.Error(err))
	}

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	metricsSrv.Shutdown(shutdownCtx)

	consumer.Close()
	log.Info("fiapx-processing-service stopped")
}

func fatalOnErr(err error, msg string) {
	if err != nil {
		panic(msg + ": " + err.Error())
	}
}
