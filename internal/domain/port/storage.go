package port

import (
	"context"
	"io"
)

type VideoStorage interface {
	DownloadVideo(ctx context.Context, objectKey string, destPath string) error
	UploadZip(ctx context.Context, objectKey string, reader io.Reader, size int64) error
}
