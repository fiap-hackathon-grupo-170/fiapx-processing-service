package port

import "context"

type Zipper interface {
	CreateZip(ctx context.Context, filePaths []string, outputPath string) error
}
