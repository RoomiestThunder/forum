package storage

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// The smallest prefixes http.DetectContentType recognises, padded so each is a
// plausible start of a file.
var (
	pngFile  = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 100)...)
	jpegFile = append([]byte("\xff\xd8\xff\xe0\x00\x10JFIF"), bytes.Repeat([]byte{0}, 100)...)
	gifFile  = append([]byte("GIF89a"), bytes.Repeat([]byte{0}, 100)...)
	webpFile = append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{0}, 100)...)
)

func TestInspectAcceptsTheAllowedImageTypes(t *testing.T) {
	for want, file := range map[string][]byte{
		"image/png":  pngFile,
		"image/jpeg": jpegFile,
		"image/gif":  gifFile,
		"image/webp": webpFile,
	} {
		got, _, _, err := inspect(bytes.NewReader(file))
		if err != nil || got != want {
			t.Errorf("inspect(%s file) = %q, %v; want %q", want, got, err, want)
		}
	}
}

// What the file is has to come from its content: the uploader controls the
// name and any header, and could otherwise store a script as an "image".
func TestInspectRejectsEverythingElse(t *testing.T) {
	for name, file := range map[string][]byte{
		"html":           []byte("<!DOCTYPE html><html><script>alert(1)</script></html>"),
		"svg":            []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"plain text":     []byte("just some text"),
		"javascript":     []byte("alert(document.cookie)"),
		"pdf":            []byte("%PDF-1.7 ..."),
		"zip":            []byte("PK\x03\x04 ..."),
		"empty":          {},
		"png text/html":  []byte("<html>\x89PNG\r\n\x1a\n"), // an image signature that isn't at the start
		"exe":            []byte("MZ\x90\x00\x03"),
		"almost a png":   []byte("\x89PNG\r\n\x1a"), // one byte short of the signature
		"gif without 89": []byte("GIF90a"),
	} {
		if got, _, _, err := inspect(bytes.NewReader(file)); !errors.Is(err, ErrUnsupportedType) {
			t.Errorf("%s: detected %q, err = %v; want ErrUnsupportedType", name, got, err)
		}
	}
}

func TestInspectReturnsTheWholeStreamIncludingTheSniffedBytes(t *testing.T) {
	file := append(append([]byte(nil), pngFile...), bytes.Repeat([]byte("x"), 5000)...) // longer than the 512-byte sniff

	_, body, _, err := inspect(bytes.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	if err != nil || !bytes.Equal(got, file) {
		t.Fatalf("the reader must reproduce the upload exactly (got %d bytes, want %d, err %v)", len(got), len(file), err)
	}
}

func TestInspectHandlesAFileShorterThanTheSniffWindow(t *testing.T) {
	tiny := []byte("GIF89a")
	_, body, _, err := inspect(bytes.NewReader(tiny))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(body); !bytes.Equal(got, tiny) {
		t.Fatalf("got %q", got)
	}
}

func TestSizeLimitAcceptsExactlyTheMaximum(t *testing.T) {
	atLimit := append(append([]byte(nil), pngFile...), make([]byte, maxUploadSize-len(pngFile))...)
	if len(atLimit) != maxUploadSize {
		t.Fatalf("test setup: %d != %d", len(atLimit), maxUploadSize)
	}

	_, body, limited, err := inspect(bytes.NewReader(atLimit))
	if err != nil {
		t.Fatal(err)
	}
	n, _ := io.Copy(io.Discard, body)
	if limited.exceeded || n != maxUploadSize {
		t.Fatalf("a file of exactly %d bytes must be accepted whole: read %d, exceeded=%v", maxUploadSize, n, limited.exceeded)
	}
}

func TestSizeLimitFlagsOneByteOver(t *testing.T) {
	over := append(append([]byte(nil), pngFile...), make([]byte, maxUploadSize-len(pngFile)+1)...)

	_, body, limited, err := inspect(bytes.NewReader(over))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, body)
	if !limited.exceeded {
		t.Fatal("a file one byte over the maximum must be flagged as too large")
	}
}

// A malicious client can claim any size; the cap has to hold for a stream that
// never ends, without reading it all.
func TestSizeLimitStopsAnEndlessStream(t *testing.T) {
	endless := io.MultiReader(bytes.NewReader(pngFile), zeroReader{})

	_, body, limited, err := inspect(endless)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := io.Copy(io.Discard, body)

	if !limited.exceeded {
		t.Fatal("an endless stream must hit the limit")
	}
	if n > maxUploadSize+1 {
		t.Fatalf("read %d bytes, more than the cap of %d", n, maxUploadSize+1)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestEveryAllowedTypeHasAnExtension(t *testing.T) {
	for contentType, ext := range allowedTypes {
		if !strings.HasPrefix(ext, ".") || strings.ContainsAny(ext, "/\\ ") || len(ext) < 2 {
			t.Errorf("%s maps to a malformed extension %q", contentType, ext)
		}
	}
}

func TestPublicBase(t *testing.T) {
	tests := []struct {
		name                string
		publicURL, endpoint string
		bucket              string
		useSSL              bool
		want                string
	}{
		{"explicit public URL wins", "https://cdn.example.com/forum", "minio:9000", "forum", false, "https://cdn.example.com/forum"},
		{"trailing slash is trimmed", "https://cdn.example.com/forum/", "minio:9000", "forum", false, "https://cdn.example.com/forum"},
		{"derived over http", "", "localhost:9000", "forum", false, "http://localhost:9000/forum"},
		{"derived over https follows useSSL", "", "s3.example.com", "forum", true, "https://s3.example.com/forum"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicBase(tt.publicURL, tt.endpoint, tt.bucket, tt.useSSL); got != tt.want {
				t.Errorf("publicBase = %q, want %q", got, tt.want)
			}
		})
	}
}
