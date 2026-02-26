package ffmpeg

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type ZipCreator struct{}

func NewZipCreator() *ZipCreator {
	return &ZipCreator{}
}

func (z *ZipCreator) CreateZip(ctx context.Context, filePaths []string, outputPath string) error {
	zipFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, fp := range filePaths {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := addFileToZip(zipWriter, fp); err != nil {
			return fmt.Errorf("add %s to zip: %w", fp, err)
		}
	}

	return nil
}

func addFileToZip(zw *zip.Writer, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	header.Name = filepath.Base(filename)
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}
