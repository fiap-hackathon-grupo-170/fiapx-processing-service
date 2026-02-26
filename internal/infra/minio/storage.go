package minio

import (
	"context"
	"fmt"
	"io"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage struct {
	client       *miniogo.Client
	uploadBucket string
	zipBucket    string
}

type StorageConfig struct {
	Endpoint     string
	AccessKey    string
	SecretKey    string
	UseSSL       bool
	UploadBucket string
	ZipBucket    string
}

func NewStorage(cfg StorageConfig) (*Storage, error) {
	client, err := miniogo.New(cfg.Endpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &Storage{
		client:       client,
		uploadBucket: cfg.UploadBucket,
		zipBucket:    cfg.ZipBucket,
	}, nil
}

func (s *Storage) EnsureBuckets(ctx context.Context) error {
	for _, bucket := range []string{s.uploadBucket, s.zipBucket} {
		exists, err := s.client.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("check bucket %s: %w", bucket, err)
		}
		if !exists {
			if err := s.client.MakeBucket(ctx, bucket, miniogo.MakeBucketOptions{}); err != nil {
				return fmt.Errorf("create bucket %s: %w", bucket, err)
			}
		}
	}
	return nil
}

func (s *Storage) DownloadVideo(ctx context.Context, objectKey string, destPath string) error {
	return s.client.FGetObject(ctx, s.uploadBucket, objectKey, destPath, miniogo.GetObjectOptions{})
}

func (s *Storage) UploadZip(ctx context.Context, objectKey string, reader io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.zipBucket, objectKey, reader, size, miniogo.PutObjectOptions{
		ContentType: "application/zip",
	})
	if err != nil {
		return fmt.Errorf("upload zip: %w", err)
	}
	return nil
}
