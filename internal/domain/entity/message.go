package entity

import "github.com/google/uuid"

// VideoProcessingMessage is the inbound message from the video.processing queue.
type VideoProcessingMessage struct {
	JobID     uuid.UUID `json:"job_id"`
	UserID    string    `json:"user_id"`
	VideoKey  string    `json:"video_key"`
	FileSize  int64     `json:"file_size"`
	UserEmail string    `json:"user_email"`
}

// VideoStatusMessage is the outbound message published to the video.status queue.
type VideoStatusMessage struct {
	JobID        uuid.UUID `json:"job_id"`
	UserID       string    `json:"user_id"`
	Status       JobStatus `json:"status"`
	VideoKey     string    `json:"video_key"`
	ZipKey       string    `json:"zip_key,omitempty"`
	FrameCount   int       `json:"frame_count,omitempty"`
	Duration     float64   `json:"duration_seconds,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	Attempt      int       `json:"attempt"`
	MaxAttempts  int       `json:"max_attempts"`
}
