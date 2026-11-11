// Package storage stores user-uploaded images in an S3-compatible bucket
// (MinIO in development and in docker-compose).
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"forum/internal/metrics"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const maxUploadSize = 5 * 1024 * 1024 // 5 MB

// Upload errors that are the uploader's doing, as opposed to a storage
// failure; callers use them to choose between a 4xx and a 5xx.
var (
	ErrUnsupportedType = errors.New("unsupported file type")
	ErrTooLarge        = errors.New("file too large")
)

// allowedTypes maps each accepted content type to the extension its objects are
// stored under. The extension comes from the detected type, never from the
// uploader's filename, which is arbitrary text.
var allowedTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

type Client struct {
	minio      *minio.Client
	bucketName string
	// publicBase is the URL prefix under which objects can be fetched by a
	// browser, without a trailing slash.
	publicBase string
}

// New connects to the bucket, creating it if needed.
//
// publicURL is where browsers reach the bucket's objects, e.g.
// "https://cdn.example.com/forum" or "http://localhost:9000/forum". It has to
// be separate from the endpoint because they often differ: inside
// docker-compose the app talks to "minio:9000", a name no browser can resolve.
// When empty it is derived from the endpoint and useSSL, which is right when
// the endpoint is directly reachable.
func New(endpoint, accessKey, secretKey, bucket string, useSSL bool, publicURL string) (*Client, error) {
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
		_ = mc.SetBucketPolicy(ctx, bucket, policy)
	}

	return &Client{minio: mc, bucketName: bucket, publicBase: publicBase(publicURL, endpoint, bucket, useSSL)}, nil
}

// publicBase returns the URL prefix objects are served from.
func publicBase(publicURL, endpoint, bucket string, useSSL bool) string {
	if publicURL != "" {
		return strings.TrimRight(publicURL, "/")
	}
	scheme := "http"
	if useSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s", scheme, endpoint, bucket)
}

// Upload validates, streams and stores an image, returning its object key,
// detected content type, public URL and measured size.
//
// The type is decided from the file's own first bytes, not from its name or any
// header the client sent, and the size is counted as it streams rather than
// taken from client-supplied metadata (hence the -1 passed to PutObject).
func (c *Client) Upload(ctx context.Context, r io.Reader) (objectKey, contentType, url string, size int64, err error) {
	contentType, body, limited, err := inspect(r)
	if err != nil {
		return "", "", "", 0, err
	}

	objectKey = "uploads/" + uuid.NewString() + allowedTypes[contentType]
	info, err := c.minio.PutObject(ctx, c.bucketName, objectKey, body, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if limited.exceeded {
		// Too large: PutObject has stored the first max+1 bytes by now. Remove them.
		_ = c.minio.RemoveObject(ctx, c.bucketName, objectKey, minio.RemoveObjectOptions{})
		return "", "", "", 0, fmt.Errorf("%w: the limit is %d MB", ErrTooLarge, maxUploadSize>>20)
	}
	if err != nil {
		return "", "", "", 0, fmt.Errorf("put object: %w", err)
	}
	metrics.UploadSizeBytes.Observe(float64(info.Size))

	return objectKey, contentType, c.publicBase + "/" + objectKey, info.Size, nil
}

// inspect reads the start of r to detect its content type and, if that is an
// allowed one, returns a reader over the whole stream (the sniffed bytes
// included), capped at maxUploadSize. limited.exceeded reports whether the cap
// was hit once the stream has been consumed.
func inspect(r io.Reader) (contentType string, body io.Reader, limited *limitedReader, err error) {
	// One byte beyond the maximum, so that a file of exactly maxUploadSize bytes
	// is accepted and one a byte larger is detected as too large.
	limited = &limitedReader{r: r, remaining: maxUploadSize + 1}

	// 512 bytes is what http.DetectContentType looks at.
	head := make([]byte, 512)
	n, err := io.ReadFull(limited, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", nil, nil, fmt.Errorf("read upload: %w", err)
	}
	head = head[:n]

	contentType = http.DetectContentType(head)
	if _, ok := allowedTypes[contentType]; !ok {
		return "", nil, nil, fmt.Errorf("%w: only JPEG, PNG, GIF and WebP images are accepted", ErrUnsupportedType)
	}
	return contentType, io.MultiReader(bytes.NewReader(head), limited), limited, nil
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
