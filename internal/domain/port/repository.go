package port

import (
	"context"

	"github.com/fiapx/fiapx-processing-service/internal/domain/entity"
	"github.com/google/uuid"
)

type JobRepository interface {
	Create(ctx context.Context, job *entity.Job) error
	Update(ctx context.Context, job *entity.Job) error
	FindByID(ctx context.Context, id uuid.UUID) (*entity.Job, error)
}
