package port

import "context"

type FrameExtractionResult struct {
	FramePaths    []string
	FrameCount    int
	VideoDuration float64
}

type FrameExtractor interface {
	ExtractFrames(ctx context.Context, videoPath string, outputDir string) (*FrameExtractionResult, error)
}
