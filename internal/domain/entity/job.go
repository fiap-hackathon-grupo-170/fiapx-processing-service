package entity

import (
	"time"

	"github.com/google/uuid"
)

type JobStatus string

const (
	JobStatusPending    JobStatus = "PENDING"
	JobStatusProcessing JobStatus = "PROCESSING"
	JobStatusCompleted  JobStatus = "COMPLETED"
	JobStatusFailed     JobStatus = "FAILED"
)

type Job struct {
	ID            uuid.UUID
	UserID        string
	VideoKey      string
	ZipKey        string
	Status        JobStatus
	FrameCount    int
	FileSize      int64
	VideoDuration float64
	Attempt       int
	MaxAttempts   int
	ErrorMessage  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CompletedAt   *time.Time
}

func NewJob(userID, videoKey string, fileSize int64, maxAttempts int) *Job {
	now := time.Now().UTC()
	return &Job{
		ID:          uuid.New(),
		UserID:      userID,
		VideoKey:    videoKey,
		FileSize:    fileSize,
		Status:      JobStatusPending,
		Attempt:     0,
		MaxAttempts: maxAttempts,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (j *Job) MarkProcessing() {
	j.Status = JobStatusProcessing
	j.Attempt++
	j.UpdatedAt = time.Now().UTC()
}

func (j *Job) MarkCompleted(zipKey string, frameCount int, duration float64) {
	now := time.Now().UTC()
	j.Status = JobStatusCompleted
	j.ZipKey = zipKey
	j.FrameCount = frameCount
	j.VideoDuration = duration
	j.UpdatedAt = now
	j.CompletedAt = &now
}

func (j *Job) MarkFailed(errMsg string) {
	j.Status = JobStatusFailed
	j.ErrorMessage = errMsg
	j.UpdatedAt = time.Now().UTC()
}

func (j *Job) CanRetry() bool {
	return j.Attempt < j.MaxAttempts
}
