package config

import (
	"github.com/caarlos0/env/v11"
)

type Config struct {
	RabbitMQURL             string `env:"RABBITMQ_URL"              envDefault:"amqp://guest:guest@rabbitmq:5672/"`
	RabbitMQProcessingQueue string `env:"RABBITMQ_PROCESSING_QUEUE" envDefault:"video.processing"`
	RabbitMQStatusQueue     string `env:"RABBITMQ_STATUS_QUEUE"     envDefault:"video.status"`
	RabbitMQDLQ             string `env:"RABBITMQ_DLQ"              envDefault:"video.processing.dlq"`
	RabbitMQExchange        string `env:"RABBITMQ_EXCHANGE"         envDefault:"fiapx.video"`
	RabbitMQPrefetch        int    `env:"RABBITMQ_PREFETCH"         envDefault:"5"`

	MinIOEndpoint     string `env:"MINIO_ENDPOINT"      envDefault:"minio:9000"`
	MinIOAccessKey    string `env:"MINIO_ACCESS_KEY"     envDefault:"minioadmin"`
	MinIOSecretKey    string `env:"MINIO_SECRET_KEY"     envDefault:"minioadmin"`
	MinIOUseSSL       bool   `env:"MINIO_USE_SSL"        envDefault:"false"`
	MinIOUploadBucket string `env:"MINIO_UPLOAD_BUCKET"  envDefault:"uploads"`
	MinIOZipBucket    string `env:"MINIO_ZIP_BUCKET"     envDefault:"zips"`

	DatabaseURL string `env:"DATABASE_URL" envDefault:"postgresql://job_user:job_pass@postgres-jobs:5432/jobs?sslmode=disable"`

	WorkerCount      int `env:"WORKER_COUNT"              envDefault:"3"`
	MaxRetries       int `env:"WORKER_MAX_RETRIES"        envDefault:"7"`
	RetryBaseDelayMs int `env:"WORKER_RETRY_BASE_DELAY_MS" envDefault:"1000"`

	FFmpegFPS    int    `env:"FFMPEG_FPS"    envDefault:"1"`
	FFmpegFormat string `env:"FFMPEG_FORMAT" envDefault:"png"`

	SMTPHost       string `env:"SMTP_HOST"        envDefault:"mailhog"`
	SMTPPort       int    `env:"SMTP_PORT"        envDefault:"1025"`
	SMTPFrom       string `env:"SMTP_FROM"        envDefault:"noreply@fiapx.local"`
	NotificationTo string `env:"NOTIFICATION_TO"  envDefault:"admin@fiapx.local"`

	MetricsPort    int    `env:"METRICS_PORT"     envDefault:"8083"`
	JaegerEndpoint string `env:"JAEGER_ENDPOINT"  envDefault:"http://jaeger:4318/v1/traces"`
	LogLevel       string `env:"LOG_LEVEL"        envDefault:"info"`

	TempDir string `env:"TEMP_DIR" envDefault:"/tmp/fiapx"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
