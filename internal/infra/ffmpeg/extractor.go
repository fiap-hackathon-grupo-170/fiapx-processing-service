package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fiapx/fiapx-processing-service/internal/domain/port"
	"go.uber.org/zap"
)

type Extractor struct {
	fps    int
	format string
	logger *zap.Logger
}

func NewExtractor(fps int, format string, logger *zap.Logger) *Extractor {
	return &Extractor{fps: fps, format: format, logger: logger}
}

func (e *Extractor) ExtractFrames(ctx context.Context, videoPath string, outputDir string) (*port.FrameExtractionResult, error) {
	duration, err := e.getVideoDuration(ctx, videoPath)
	if err != nil {
		e.logger.Warn("could not get video duration", zap.Error(err))
	}

	framePattern := filepath.Join(outputDir, fmt.Sprintf("frame_%%04d.%s", e.format))
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=%d", e.fps),
		"-y",
		framePattern,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg error: %w, output: %s", err, string(output))
	}

	globPattern := filepath.Join(outputDir, fmt.Sprintf("*.%s", e.format))
	frames, err := filepath.Glob(globPattern)
	if err != nil {
		return nil, fmt.Errorf("glob frames: %w", err)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames extracted from video")
	}

	e.logger.Info("frames extracted",
		zap.Int("count", len(frames)),
		zap.Float64("video_duration", duration),
	)

	return &port.FrameExtractionResult{
		FramePaths:    frames,
		FrameCount:    len(frames),
		VideoDuration: duration,
	}, nil
}

func (e *Extractor) getVideoDuration(ctx context.Context, videoPath string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return duration, nil
}
