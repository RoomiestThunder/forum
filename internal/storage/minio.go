package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"forum/internal/metrics"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const maxUploadSize = 5 * 1024 * 1024 // 5 MB

var allowedTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

type Client struct {
	minio      *minio.Client
	bucketName string
}

func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	ctx := context.Background()
	exists, err := mc.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket: %w", err)
	}
	if !exists {
		if err := mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket: %w", err)
		}
		// Make bucket publicly readable for images
		policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, bucket)
		mc.SetBucketPolicy(ctx, bucket, policy)
	}

	return &Client{minio: mc, bucketName: bucket}, nil
}

// Upload validates, streams, and stores a file.
// Pass size=-1 to let MinIO measure the actual byte count (recommended; avoids
// trusting caller-supplied metadata). Returns (objectKey, contentType, url, actualSize, error).
func (c *Client) Upload(ctx context.Context, r io.Reader, filename string, size int64) (objectKey, contentType, url string, actualSize int64, err error) {
	// Wrap reader to enforce the hard size limit regardless of declared size.
	limited := &limitedReader{r: r, remaining: maxUploadSize + 1}

	// Read first 512 bytes for magic-byte content-type detection.
	buf := make([]byte, 512)
	n, err := limited.Read(buf)
	if err != nil && err != io.EOF {
		return "", "", "", 0, fmt.Errorf("read header: %w", err)
	}
	buf = buf[:n]
	contentType = http.DetectContentType(buf)

	if !allowedTypes[contentType] {
		return "", "", "", 0, fmt.Errorf("unsupported file type: %s", contentType)
	}

	// Reconstruct the reader with consumed bytes prepended.
	combined := io.MultiReader(bytes.NewReader(buf), limited)

	ext := strings.ToLower(filepath.Ext(filename))
	objectKey = fmt.Sprintf("uploads/%d%s", time.Now().UnixNano(), ext)

	// Pass -1 so MinIO measures actual bytes; size parameter is untrusted.
	info, err := c.minio.PutObject(ctx, c.bucketName, objectKey, combined, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if limited.exceeded {
		c.minio.RemoveObject(ctx, c.bucketName, objectKey, minio.RemoveObjectOptions{})
		return "", "", "", 0, fmt.Errorf("file too large (max 5MB)")
	}
	if err != nil {
		return "", "", "", 0, fmt.Errorf("put object: %w", err)
	}
	actualSize = info.Size
	metrics.UploadSizeBytes.Observe(float64(actualSize))

	url = fmt.Sprintf("http://%s/%s/%s", c.minio.EndpointURL().Host, c.bucketName, objectKey)
	return objectKey, contentType, url, actualSize, nil
}

// PresignedURL generates a temporary download URL.
func (c *Client) PresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	url, err := c.minio.PresignedGetObject(ctx, c.bucketName, objectKey, expiry, nil)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

// Delete removes an object from storage.
func (c *Client) Delete(ctx context.Context, objectKey string) error {
	return c.minio.RemoveObject(ctx, c.bucketName, objectKey, minio.RemoveObjectOptions{})
}

// limitedReader enforces a hard byte cap on the underlying reader.
type limitedReader struct {
	r         io.Reader
	remaining int64
	exceeded  bool
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		l.exceeded = true
		return 0, io.EOF
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.r.Read(p)
	l.remaining -= int64(n)
	if l.remaining <= 0 {
		l.exceeded = true
	}
	return n, err
}
