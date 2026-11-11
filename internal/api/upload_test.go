package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"forum/internal/database"
	apperrors "forum/internal/errors"
	"forum/internal/storage"
)

// fakeFiles stands in for the object store.
type fakeFiles struct {
	mu        sync.Mutex
	uploadErr error
	uploads   int
	deleted   []string
}

func (f *fakeFiles) Upload(ctx context.Context, r io.Reader) (string, string, string, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uploadErr != nil {
		return "", "", "", 0, f.uploadErr
	}
	data, _ := io.ReadAll(r)
	f.uploads++
	key := fmt.Sprintf("uploads/fake-%d.png", f.uploads)
	return key, "image/png", "https://files.example/forum/" + key, int64(len(data)), nil
}

func (f *fakeFiles) Delete(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, key)
	return nil
}

func (f *fakeFiles) deletedKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

// failingWrites makes the database refuse to record an upload or an avatar.
type failingWrites struct{ database.Store }

func (failingWrites) CreateUpload(int, string, string, string, int64, string) (int, error) {
	return 0, apperrors.DatabaseError(errors.New("disk full"))
}
func (failingWrites) UpdateUserAvatar(int, string) error {
	return apperrors.DatabaseError(errors.New("disk full"))
}

func newUploadServer(t *testing.T, files *fakeFiles, configure ...func(*Server)) *httptest.Server {
	t.Helper()
	srv, _ := newTestServerFull(t, Options{BaseURL: "http://localhost"}, nil, append([]func(*Server){func(s *Server) {
		if files != nil {
			s.storage = files
		}
	}}, configure...)...)
	return srv
}

func multipartBody(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(content)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func postFile(t *testing.T, url, token, field, filename string, content []byte) *http.Response {
	t.Helper()
	body, contentType := multipartBody(t, field, filename, content)
	req, _ := http.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func errorMessage(t *testing.T, resp *http.Response) string {
	t.Helper()
	return decode[map[string]string](t, resp)["error"]
}

func TestUploadStoresTheFileAndRecordsIt(t *testing.T) {
	files := &fakeFiles{}
	srv, store := newTestServerFull(t, Options{BaseURL: "http://localhost"}, nil, func(s *Server) { s.storage = files })
	alice := registerUser(t, srv.URL, "alice")

	resp := postFile(t, srv.URL+"/api/v1/upload", alice, "file", "holiday.png", []byte("pretend png"))

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: want 201, got %d", resp.StatusCode)
	}
	body := decode[map[string]any](t, resp)
	if url, _ := body["url"].(string); !strings.HasPrefix(url, "https://files.example/forum/uploads/") {
		t.Errorf("url = %v", body["url"])
	}
	uploads, err := store.GetUploadsByUser(userIDOf(t, srv.URL, alice))
	if err != nil || len(uploads) != 1 || uploads[0].Filename != "holiday.png" {
		t.Fatalf("the upload should be recorded against the user: %+v (%v)", uploads, err)
	}
}

func TestUploadErrorsAreClassified(t *testing.T) {
	tests := []struct {
		name      string
		storeErr  error
		wantCode  int
		wantInMsg string
		forbidden string
	}{
		{"unsupported type", fmt.Errorf("%w: only images", storage.ErrUnsupportedType), http.StatusUnsupportedMediaType, "unsupported file type", ""},
		{"too large", fmt.Errorf("%w: the limit is 5 MB", storage.ErrTooLarge), http.StatusRequestEntityTooLarge, "too large", ""},
		{
			"storage failure", errors.New(`put object: Put "http://minio-internal:9000/forum/x": dial tcp 10.1.2.3:9000: refused`),
			http.StatusInternalServerError, "failed to upload file", "minio-internal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newUploadServer(t, &fakeFiles{uploadErr: tt.storeErr}, func(s *Server) {
				s.logger = quietLogger()
			})
			alice := registerUser(t, srv.URL, "alice")

			resp := postFile(t, srv.URL+"/api/v1/upload", alice, "file", "x.png", []byte("data"))

			if resp.StatusCode != tt.wantCode {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantCode)
			}
			msg := errorMessage(t, resp)
			if !strings.Contains(msg, tt.wantInMsg) {
				t.Errorf("message %q should contain %q", msg, tt.wantInMsg)
			}
			if tt.forbidden != "" && strings.Contains(msg, tt.forbidden) {
				t.Errorf("message %q leaks storage internals (%q)", msg, tt.forbidden)
			}
		})
	}
}

func TestUploadWithoutConfiguredStorage(t *testing.T) {
	srv := newUploadServer(t, nil)
	alice := registerUser(t, srv.URL, "alice")

	if got := postFile(t, srv.URL+"/api/v1/upload", alice, "file", "x.png", []byte("data")).StatusCode; got != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", got)
	}
	if got := postFile(t, srv.URL+"/api/v1/me/avatar", alice, "file", "x.png", []byte("data")).StatusCode; got != http.StatusServiceUnavailable {
		t.Fatalf("avatar: want 503, got %d", got)
	}
}

func TestUploadRequiresLogin(t *testing.T) {
	srv := newUploadServer(t, &fakeFiles{})
	for _, path := range []string{"/api/v1/upload", "/api/v1/me/avatar"} {
		if got := postFile(t, srv.URL+path, "", "file", "x.png", []byte("data")).StatusCode; got != http.StatusUnauthorized {
			t.Errorf("%s without a token: want 401, got %d", path, got)
		}
	}
}

func TestUploadRejectsMalformedRequests(t *testing.T) {
	srv := newUploadServer(t, &fakeFiles{})
	alice := registerUser(t, srv.URL, "alice")

	if got := postFile(t, srv.URL+"/api/v1/upload", alice, "attachment", "x.png", []byte("data")).StatusCode; got != http.StatusBadRequest {
		t.Errorf("a file in the wrong field: want 400, got %d", got)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/upload", strings.NewReader(`{"not":"multipart"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+alice)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a non-multipart body: want 400, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestOversizedBodyIsRefusedWithoutReadingItAll(t *testing.T) {
	files := &fakeFiles{}
	srv := newUploadServer(t, files)
	alice := registerUser(t, srv.URL, "alice")

	resp := postFile(t, srv.URL+"/api/v1/upload", alice, "file", "big.png", bytes.Repeat([]byte("x"), uploadBodyLimit+1))

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", resp.StatusCode)
	}
	if files.uploads != 0 {
		t.Fatal("nothing should have reached the store")
	}
}

// If the file is stored but recording it fails, the file would be left in the
// bucket with nothing pointing at it.
func TestStoredFileIsRemovedWhenItCannotBeRecorded(t *testing.T) {
	files := &fakeFiles{}
	srv := newUploadServer(t, files, func(s *Server) {
		s.store = failingWrites{s.store}
		s.logger = quietLogger()
	})
	alice := registerUser(t, srv.URL, "alice")

	for name, path := range map[string]string{"upload": "/api/v1/upload", "avatar": "/api/v1/me/avatar"} {
		resp := postFile(t, srv.URL+path, alice, "file", "x.png", []byte("data"))
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("%s: want 500, got %d", name, resp.StatusCode)
		}
		if msg := errorMessage(t, resp); strings.Contains(msg, "disk full") {
			t.Errorf("%s: the database error leaked to the client: %q", name, msg)
		}
	}
	if got := len(files.deletedKeys()); got != 2 {
		t.Fatalf("both stored files should have been removed, got %v", files.deletedKeys())
	}
}

func TestAvatarUploadSetsTheUsersAvatar(t *testing.T) {
	srv := newUploadServer(t, &fakeFiles{})
	alice := registerUser(t, srv.URL, "alice")

	resp := postFile(t, srv.URL+"/api/v1/me/avatar", alice, "file", "me.png", []byte("data"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("avatar: want 200, got %d", resp.StatusCode)
	}
	url := decode[map[string]string](t, resp)["avatar_url"]

	me := decode[map[string]any](t, doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", alice, nil))
	if me["avatar_url"] != url || url == "" {
		t.Fatalf("/me avatar_url = %v, want %q", me["avatar_url"], url)
	}
}

func TestUploadsAreRateLimitedPerUser(t *testing.T) {
	srv := newUploadServer(t, &fakeFiles{})
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")

	for i := 0; i < limitUpload.limit; i++ {
		if got := postFile(t, srv.URL+"/api/v1/upload", alice, "file", "x.png", []byte("data")).StatusCode; got != http.StatusCreated {
			t.Fatalf("upload %d: want 201, got %d", i+1, got)
		}
	}
	if got := postFile(t, srv.URL+"/api/v1/upload", alice, "file", "x.png", []byte("data")).StatusCode; got != http.StatusTooManyRequests {
		t.Errorf("past the limit: want 429, got %d", got)
	}
	if got := postFile(t, srv.URL+"/api/v1/upload", bob, "file", "x.png", []byte("data")).StatusCode; got != http.StatusCreated {
		t.Errorf("another user has their own allowance: got %d", got)
	}
}
