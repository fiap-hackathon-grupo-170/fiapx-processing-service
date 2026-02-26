package integration

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fiapx/fiapx-processing-service/internal/domain/entity"
	"github.com/fiapx/fiapx-processing-service/internal/infra/email"
	"github.com/fiapx/fiapx-processing-service/internal/infra/ffmpeg"
	miniostorage "github.com/fiapx/fiapx-processing-service/internal/infra/minio"
	"github.com/fiapx/fiapx-processing-service/internal/infra/postgres"
	"github.com/fiapx/fiapx-processing-service/internal/infra/rabbitmq"
	"github.com/fiapx/fiapx-processing-service/internal/usecase"
	"github.com/fiapx/fiapx-processing-service/pkg/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	tcrabbitmq "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

func TestProcessVideoEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start PostgreSQL container
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:15-alpine",
		tcpostgres.WithDatabase("jobs"),
		tcpostgres.WithUsername("job_user"),
		tcpostgres.WithPassword("job_pass"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	defer pgContainer.Terminate(ctx)

	pgConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Start RabbitMQ container
	rmqContainer, err := tcrabbitmq.Run(ctx,
		"rabbitmq:3.12-management-alpine",
	)
	require.NoError(t, err)
	defer rmqContainer.Terminate(ctx)

	rmqURL, err := rmqContainer.AmqpURL(ctx)
	require.NoError(t, err)

	// Start MinIO container
	minioContainer, err := tcminio.Run(ctx,
		"minio/minio:latest",
		tcminio.WithUsername("minioadmin"),
		tcminio.WithPassword("minioadmin"),
	)
	require.NoError(t, err)
	defer minioContainer.Terminate(ctx)

	minioEndpoint, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Run migrations
	err = postgres.RunMigrations(pgConnStr, "../../migrations")
	require.NoError(t, err)

	// Setup MinIO storage
	storage, err := miniostorage.NewStorage(miniostorage.StorageConfig{
		Endpoint:     minioEndpoint,
		AccessKey:    "minioadmin",
		SecretKey:    "minioadmin",
		UseSSL:       false,
		UploadBucket: "uploads",
		ZipBucket:    "zips",
	})
	require.NoError(t, err)
	require.NoError(t, storage.EnsureBuckets(ctx))

	// Upload test video to MinIO
	testVideoPath := filepath.Join("..", "testdata", "test.mp4")
	if _, err := os.Stat(testVideoPath); os.IsNotExist(err) {
		t.Skip("test video not found at tests/testdata/test.mp4 - generate it with: ffmpeg -f lavfi -i testsrc=duration=2:size=320x240:rate=1 -c:v libx264 -pix_fmt yuv420p tests/testdata/test.mp4")
	}

	minioClient, err := miniogo.New(minioEndpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	require.NoError(t, err)

	videoKey := "testuser/test.mp4"
	_, err = minioClient.FPutObject(ctx, "uploads", videoKey, testVideoPath, miniogo.PutObjectOptions{
		ContentType: "video/mp4",
	})
	require.NoError(t, err)

	// Setup RabbitMQ publisher connection
	rmqConn, err := amqp.Dial(rmqURL)
	require.NoError(t, err)
	defer rmqConn.Close()

	pub, err := rabbitmq.NewPublisher(rmqConn, "fiapx.video")
	require.NoError(t, err)

	statusPub := rabbitmq.NewStatusPublisher(pub)
	dlqPub := rabbitmq.NewDLQPublisher(pub, "video.processing.dlq")

	// Setup DB pool
	pool, err := pgxpool.New(ctx, pgConnStr)
	require.NoError(t, err)
	defer pool.Close()

	// Setup use case
	log, _ := logger.New("debug")
	repo := postgres.NewJobRepository(pool)
	extractor := ffmpeg.NewExtractor(1, "png", log)
	zipper := ffmpeg.NewZipCreator()
	notifier := email.NewSMTPNotifier("localhost", 1025, "test@test.local", log)

	uc := usecase.NewProcessVideoUseCase(
		repo, storage, extractor, zipper,
		statusPub, dlqPub, notifier,
		log,
		usecase.ProcessVideoConfig{
			TempDir:    t.TempDir(),
			MaxRetries: 3,
		},
	)

	// Setup consumer
	consumer, err := rabbitmq.NewConsumer(rabbitmq.ConsumerConfig{
		URL:         rmqURL,
		Queue:       "video.processing",
		Exchange:    "fiapx.video",
		DLQ:         "video.processing.dlq",
		StatusQueue: "video.status",
		Prefetch:    1,
		WorkerCount: 1,
		BaseDelayMs: 100,
	}, uc.Execute, log)
	require.NoError(t, err)
	defer consumer.Close()

	// Start consumer in background
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	defer consumerCancel()

	go func() {
		consumer.Start(consumerCtx)
	}()

	// Give consumer time to start
	time.Sleep(500 * time.Millisecond)

	// Publish processing message
	jobID := uuid.New()
	videoInfo, _ := os.Stat(testVideoPath)
	processingMsg := entity.VideoProcessingMessage{
		JobID:     jobID,
		UserID:    "testuser",
		VideoKey:  videoKey,
		FileSize:  videoInfo.Size(),
		UserEmail: "test@test.local",
	}
	msgBody, err := json.Marshal(processingMsg)
	require.NoError(t, err)

	// Publish to processing queue via the exchange
	pubCh, err := rmqConn.Channel()
	require.NoError(t, err)
	err = pubCh.PublishWithContext(ctx,
		"fiapx.video",
		"video.processing",
		false, false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        msgBody,
		},
	)
	require.NoError(t, err)
	pubCh.Close()

	// Wait for status message on video.status queue
	statusCh, err := rmqConn.Channel()
	require.NoError(t, err)
	defer statusCh.Close()

	statusMsgs, err := statusCh.Consume("video.status", "", true, false, false, false, nil)
	require.NoError(t, err)

	var statusMsg entity.VideoStatusMessage
	select {
	case delivery := <-statusMsgs:
		err = json.Unmarshal(delivery.Body, &statusMsg)
		require.NoError(t, err)
	case <-time.After(2 * time.Minute):
		t.Fatal("timeout waiting for status message")
	}

	// Assert status
	assert.Equal(t, jobID, statusMsg.JobID)
	assert.Equal(t, entity.JobStatusCompleted, statusMsg.Status)
	assert.Greater(t, statusMsg.FrameCount, 0)
	assert.NotEmpty(t, statusMsg.ZipKey)

	// Verify ZIP exists in MinIO
	zipObj, err := minioClient.GetObject(ctx, "zips", statusMsg.ZipKey, miniogo.GetObjectOptions{})
	require.NoError(t, err)

	// Download and verify ZIP contents
	tmpZip := filepath.Join(t.TempDir(), "result.zip")
	tmpFile, err := os.Create(tmpZip)
	require.NoError(t, err)
	_, err = tmpFile.ReadFrom(zipObj)
	require.NoError(t, err)
	tmpFile.Close()

	zipReader, err := zip.OpenReader(tmpZip)
	require.NoError(t, err)
	defer zipReader.Close()

	pngCount := 0
	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, ".png") {
			pngCount++
		}
	}
	assert.Greater(t, pngCount, 0, "ZIP should contain PNG frames")
	assert.Equal(t, statusMsg.FrameCount, pngCount)

	// Verify job record in database
	var dbStatus string
	var dbFrameCount int
	err = pool.QueryRow(ctx,
		"SELECT status, frame_count FROM processing_jobs WHERE id=$1", jobID,
	).Scan(&dbStatus, &dbFrameCount)
	require.NoError(t, err)
	assert.Equal(t, "COMPLETED", dbStatus)
	assert.Equal(t, pngCount, dbFrameCount)

	// Stop consumer
	consumerCancel()

	t.Logf("Test passed: %d frames extracted, ZIP at %s", pngCount, statusMsg.ZipKey)
}

func TestProcessVideoMalformedMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Start PostgreSQL
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:15-alpine",
		tcpostgres.WithDatabase("jobs"),
		tcpostgres.WithUsername("job_user"),
		tcpostgres.WithPassword("job_pass"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	defer pgContainer.Terminate(ctx)

	pgConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Start RabbitMQ
	rmqContainer, err := tcrabbitmq.Run(ctx,
		"rabbitmq:3.12-management-alpine",
	)
	require.NoError(t, err)
	defer rmqContainer.Terminate(ctx)

	rmqURL, err := rmqContainer.AmqpURL(ctx)
	require.NoError(t, err)

	// Run migrations
	err = postgres.RunMigrations(pgConnStr, "../../migrations")
	require.NoError(t, err)

	// MinIO (minimal - no video upload needed for this test)
	minioContainer, err := tcminio.Run(ctx,
		"minio/minio:latest",
		tcminio.WithUsername("minioadmin"),
		tcminio.WithPassword("minioadmin"),
	)
	require.NoError(t, err)
	defer minioContainer.Terminate(ctx)

	minioEndpoint, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err)

	storage, err := miniostorage.NewStorage(miniostorage.StorageConfig{
		Endpoint:     minioEndpoint,
		AccessKey:    "minioadmin",
		SecretKey:    "minioadmin",
		UseSSL:       false,
		UploadBucket: "uploads",
		ZipBucket:    "zips",
	})
	require.NoError(t, err)
	require.NoError(t, storage.EnsureBuckets(ctx))

	// Setup
	pool, err := pgxpool.New(ctx, pgConnStr)
	require.NoError(t, err)
	defer pool.Close()

	log, _ := logger.New("debug")
	rmqConn, err := amqp.Dial(rmqURL)
	require.NoError(t, err)
	defer rmqConn.Close()

	pub, err := rabbitmq.NewPublisher(rmqConn, "fiapx.video")
	require.NoError(t, err)

	statusPub := rabbitmq.NewStatusPublisher(pub)
	dlqPub := rabbitmq.NewDLQPublisher(pub, "video.processing.dlq")

	repo := postgres.NewJobRepository(pool)
	extractor := ffmpeg.NewExtractor(1, "png", log)
	zipper := ffmpeg.NewZipCreator()
	notifier := email.NewSMTPNotifier("localhost", 1025, "test@test.local", log)

	uc := usecase.NewProcessVideoUseCase(
		repo, storage, extractor, zipper,
		statusPub, dlqPub, notifier,
		log,
		usecase.ProcessVideoConfig{
			TempDir:    t.TempDir(),
			MaxRetries: 3,
		},
	)

	consumer, err := rabbitmq.NewConsumer(rabbitmq.ConsumerConfig{
		URL:         rmqURL,
		Queue:       "video.processing",
		Exchange:    "fiapx.video",
		DLQ:         "video.processing.dlq",
		StatusQueue: "video.status",
		Prefetch:    1,
		WorkerCount: 1,
		BaseDelayMs: 100,
	}, uc.Execute, log)
	require.NoError(t, err)
	defer consumer.Close()

	consumerCtx, consumerCancel := context.WithCancel(ctx)
	defer consumerCancel()

	go func() {
		consumer.Start(consumerCtx)
	}()
	time.Sleep(500 * time.Millisecond)

	// Publish malformed message
	pubCh, err := rmqConn.Channel()
	require.NoError(t, err)
	err = pubCh.PublishWithContext(ctx,
		"fiapx.video",
		"video.processing",
		false, false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        []byte(`{invalid json`),
		},
	)
	require.NoError(t, err)
	pubCh.Close()

	// Wait and verify message landed in DLQ
	time.Sleep(2 * time.Second)

	dlqCh, err := rmqConn.Channel()
	require.NoError(t, err)
	defer dlqCh.Close()

	dlqMsg, ok, err := dlqCh.Get("video.processing.dlq", true)
	require.NoError(t, err)
	assert.True(t, ok, "malformed message should be in DLQ")
	assert.Equal(t, `{invalid json`, string(dlqMsg.Body))

	consumerCancel()
	t.Log("Test passed: malformed message sent to DLQ")
}
