package postgres

import (
	"context"
	"fmt"

	"github.com/fiapx/fiapx-processing-service/internal/domain/entity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type JobRepository struct {
	pool *pgxpool.Pool
}

func NewJobRepository(pool *pgxpool.Pool) *JobRepository {
	return &JobRepository{pool: pool}
}

func (r *JobRepository) Create(ctx context.Context, job *entity.Job) error {
	query := `
		INSERT INTO processing_jobs (
			id, user_id, video_key, zip_key, status, frame_count,
			file_size, video_duration, attempt, max_attempts,
			error_message, created_at, updated_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`

	_, err := r.pool.Exec(ctx, query,
		job.ID, job.UserID, job.VideoKey, job.ZipKey, string(job.Status),
		job.FrameCount, job.FileSize, job.VideoDuration,
		job.Attempt, job.MaxAttempts, job.ErrorMessage,
		job.CreatedAt, job.UpdatedAt, job.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

func (r *JobRepository) Update(ctx context.Context, job *entity.Job) error {
	query := `
		UPDATE processing_jobs SET
			status=$2, zip_key=$3, frame_count=$4, video_duration=$5,
			attempt=$6, error_message=$7, updated_at=$8, completed_at=$9
		WHERE id=$1`

	_, err := r.pool.Exec(ctx, query,
		job.ID, string(job.Status), job.ZipKey, job.FrameCount,
		job.VideoDuration, job.Attempt, job.ErrorMessage,
		job.UpdatedAt, job.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}
	return nil
}

func (r *JobRepository) FindByID(ctx context.Context, id uuid.UUID) (*entity.Job, error) {
	query := `
		SELECT id, user_id, video_key, zip_key, status, frame_count,
			file_size, video_duration, attempt, max_attempts,
			error_message, created_at, updated_at, completed_at
		FROM processing_jobs WHERE id=$1`

	job := &entity.Job{}
	var status string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&job.ID, &job.UserID, &job.VideoKey, &job.ZipKey, &status,
		&job.FrameCount, &job.FileSize, &job.VideoDuration,
		&job.Attempt, &job.MaxAttempts, &job.ErrorMessage,
		&job.CreatedAt, &job.UpdatedAt, &job.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("find job by id: %w", err)
	}
	job.Status = entity.JobStatus(status)
	return job, nil
}
