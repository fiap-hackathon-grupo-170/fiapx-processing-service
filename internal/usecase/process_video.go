package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fiapx/fiapx-processing-service/internal/domain/entity"
	"github.com/fiapx/fiapx-processing-service/internal/domain/port"
	"github.com/fiapx/fiapx-processing-service/internal/infra/metrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

type ProcessVideoUseCase struct {
	repo      port.JobRepository
	storage   port.VideoStorage
	extractor port.FrameExtractor
	zipper    port.Zipper
	publisher port.StatusPublisher
	dlq       port.DLQPublisher
	notifier  port.FailureNotifier
	logger    *zap.Logger
	tempDir   string
	maxRetry  int
}

type ProcessVideoConfig struct {
	TempDir    string
	MaxRetries int
}

func NewProcessVideoUseCase(
	repo port.JobRepository,
	storage port.VideoStorage,
	extractor port.FrameExtractor,
	zipper port.Zipper,
	publisher port.StatusPublisher,
	dlq port.DLQPublisher,
	notifier port.FailureNotifier,
	logger *zap.Logger,
	cfg ProcessVideoConfig,
) *ProcessVideoUseCase {
	return &ProcessVideoUseCase{
		repo:      repo,
		storage:   storage,
		extractor: extractor,
		zipper:    zipper,
		publisher: publisher,
		dlq:       dlq,
		notifier:  notifier,
		logger:    logger,
		tempDir:   cfg.TempDir,
		maxRetry:  cfg.MaxRetries,
	}
}

func (uc *ProcessVideoUseCase) Execute(ctx context.Context, rawMsg []byte) error {
	tracer := otel.Tracer("usecase")
	ctx, span := tracer.Start(ctx, "ProcessVideoUseCase.Execute")
	defer span.End()

	totalTimer := time.Now()

	var msg entity.VideoProcessingMessage
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		uc.logger.Error("failed to unmarshal message", zap.Error(err), zap.ByteString("body", rawMsg))
		_ = uc.dlq.PublishToDLQ(ctx, rawMsg, "unmarshal_error: "+err.Error())
		return nil
	}

	span.SetAttributes(
		attribute.String("job.id", msg.JobID.String()),
		attribute.String("job.video_key", msg.VideoKey),
	)

	log := uc.logger.With(zap.String("job_id", msg.JobID.String()), zap.String("video_key", msg.VideoKey))

	job, err := uc.repo.FindByID(ctx, msg.JobID)
	if err != nil {
		job = entity.NewJob(msg.UserID, msg.VideoKey, msg.FileSize, uc.maxRetry)
		job.ID = msg.JobID
		if err := uc.repo.Create(ctx, job); err != nil {
			log.Error("failed to create job record", zap.Error(err))
			return fmt.Errorf("create job: %w", err)
		}
	}

	if !job.CanRetry() {
		log.Warn("job exhausted retries, sending to DLQ")
		_ = uc.handlePermanentFailure(ctx, job, msg, rawMsg, "max retries exceeded")
		return nil
	}

	job.MarkProcessing()
	if err := uc.repo.Update(ctx, job); err != nil {
		log.Error("failed to update job to PROCESSING", zap.Error(err))
		return fmt.Errorf("update job: %w", err)
	}

	metrics.ActiveWorkers.Inc()
	defer metrics.ActiveWorkers.Dec()

	if err := uc.processVideoPipeline(ctx, job, msg, rawMsg, log); err != nil {
		return err
	}

	metrics.JobsProcessedTotal.WithLabelValues("completed").Inc()
	metrics.JobProcessingDuration.WithLabelValues("total").Observe(time.Since(totalTimer).Seconds())

	return nil
}

func (uc *ProcessVideoUseCase) processVideoPipeline(
	ctx context.Context,
	job *entity.Job,
	msg entity.VideoProcessingMessage,
	rawMsg []byte,
	log *zap.Logger,
) error {
	tracer := otel.Tracer("usecase")

	workDir := filepath.Join(uc.tempDir, job.ID.String())
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}
	defer os.RemoveAll(workDir)

	// Download video from MinIO
	dlStart := time.Now()
	ctx2, spanDl := tracer.Start(ctx, "download_video")
	videoPath := filepath.Join(workDir, "input.mp4")
	if err := uc.storage.DownloadVideo(ctx2, msg.VideoKey, videoPath); err != nil {
		spanDl.End()
		log.Error("failed to download video", zap.Error(err))
		return uc.handleRetryableFailure(ctx, job, msg, rawMsg, "download_video: "+err.Error(), log)
	}
	spanDl.End()
	metrics.JobProcessingDuration.WithLabelValues("download").Observe(time.Since(dlStart).Seconds())

	// Extract frames with FFmpeg
	exStart := time.Now()
	ctx3, spanEx := tracer.Start(ctx, "extract_frames")
	framesDir := filepath.Join(workDir, "frames")
	if err := os.MkdirAll(framesDir, 0755); err != nil {
		spanEx.End()
		return fmt.Errorf("create frames dir: %w", err)
	}
	result, err := uc.extractor.ExtractFrames(ctx3, videoPath, framesDir)
	if err != nil {
		spanEx.End()
		log.Error("frame extraction failed", zap.Error(err))
		return uc.handleRetryableFailure(ctx, job, msg, rawMsg, "extract_frames: "+err.Error(), log)
	}
	spanEx.End()
	metrics.JobProcessingDuration.WithLabelValues("extract").Observe(time.Since(exStart).Seconds())
	metrics.FramesExtractedTotal.Add(float64(result.FrameCount))

	// Create ZIP from frames
	zipStart := time.Now()
	ctx4, spanZip := tracer.Start(ctx, "create_zip")
	zipPath := filepath.Join(workDir, "frames.zip")
	if err := uc.zipper.CreateZip(ctx4, result.FramePaths, zipPath); err != nil {
		spanZip.End()
		log.Error("zip creation failed", zap.Error(err))
		return uc.handleRetryableFailure(ctx, job, msg, rawMsg, "create_zip: "+err.Error(), log)
	}
	spanZip.End()
	metrics.JobProcessingDuration.WithLabelValues("zip").Observe(time.Since(zipStart).Seconds())

	// Upload ZIP to MinIO
	upStart := time.Now()
	ctx5, spanUp := tracer.Start(ctx, "upload_zip")
	zipKey := fmt.Sprintf("%s/frames_%s.zip", msg.UserID, job.ID.String())
	zipFile, err := os.Open(zipPath)
	if err != nil {
		spanUp.End()
		return uc.handleRetryableFailure(ctx, job, msg, rawMsg, "open_zip: "+err.Error(), log)
	}
	zipStat, _ := zipFile.Stat()
	if err := uc.storage.UploadZip(ctx5, zipKey, zipFile, zipStat.Size()); err != nil {
		zipFile.Close()
		spanUp.End()
		log.Error("zip upload failed", zap.Error(err))
		return uc.handleRetryableFailure(ctx, job, msg, rawMsg, "upload_zip: "+err.Error(), log)
	}
	zipFile.Close()
	spanUp.End()
	metrics.JobProcessingDuration.WithLabelValues("upload").Observe(time.Since(upStart).Seconds())

	// Mark completed
	job.MarkCompleted(zipKey, result.FrameCount, result.VideoDuration)
	if err := uc.repo.Update(ctx, job); err != nil {
		log.Error("failed to update job to COMPLETED", zap.Error(err))
		return fmt.Errorf("update job completed: %w", err)
	}

	uc.publishStatus(ctx, job, log)

	log.Info("job completed successfully",
		zap.Int("frame_count", result.FrameCount),
		zap.Float64("duration_secs", result.VideoDuration),
		zap.String("zip_key", zipKey),
	)

	return nil
}

func (uc *ProcessVideoUseCase) handleRetryableFailure(
	ctx context.Context,
	job *entity.Job,
	msg entity.VideoProcessingMessage,
	rawMsg []byte,
	errMsg string,
	log *zap.Logger,
) error {
	job.MarkFailed(errMsg)
	_ = uc.repo.Update(ctx, job)

	if !job.CanRetry() {
		return uc.handlePermanentFailure(ctx, job, msg, rawMsg, errMsg)
	}

	metrics.RetryTotal.WithLabelValues(strconv.Itoa(job.Attempt)).Inc()
	uc.publishStatus(ctx, job, log)

	return fmt.Errorf("retryable failure (attempt %d/%d): %s", job.Attempt, job.MaxAttempts, errMsg)
}

func (uc *ProcessVideoUseCase) handlePermanentFailure(
	ctx context.Context,
	job *entity.Job,
	msg entity.VideoProcessingMessage,
	rawMsg []byte,
	errMsg string,
) error {
	job.MarkFailed(errMsg)
	_ = uc.repo.Update(ctx, job)

	_ = uc.dlq.PublishToDLQ(ctx, rawMsg, errMsg)

	uc.publishStatus(ctx, job, uc.logger)

	metrics.JobsProcessedTotal.WithLabelValues("dlq").Inc()

	if msg.UserEmail != "" {
		_ = uc.notifier.NotifyFailure(ctx, msg.UserEmail, job.ID.String(), msg.VideoKey, errMsg)
	}

	return nil
}

func (uc *ProcessVideoUseCase) publishStatus(ctx context.Context, job *entity.Job, log *zap.Logger) {
	statusMsg := entity.VideoStatusMessage{
		JobID:        job.ID,
		UserID:       job.UserID,
		Status:       job.Status,
		VideoKey:     job.VideoKey,
		ZipKey:       job.ZipKey,
		FrameCount:   job.FrameCount,
		Duration:     job.VideoDuration,
		ErrorMessage: job.ErrorMessage,
		Attempt:      job.Attempt,
		MaxAttempts:  job.MaxAttempts,
	}
	data, _ := json.Marshal(statusMsg)
	if err := uc.publisher.PublishStatus(ctx, data); err != nil {
		log.Error("failed to publish status", zap.Error(err))
	}
}
