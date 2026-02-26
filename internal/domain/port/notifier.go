package port

import "context"

type FailureNotifier interface {
	NotifyFailure(ctx context.Context, userEmail string, jobID string, videoKey string, errorMsg string) error
}
