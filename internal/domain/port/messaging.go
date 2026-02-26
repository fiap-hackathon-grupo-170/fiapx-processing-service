package port

import "context"

type StatusPublisher interface {
	PublishStatus(ctx context.Context, msg []byte) error
}

type DLQPublisher interface {
	PublishToDLQ(ctx context.Context, msg []byte, reason string) error
}
