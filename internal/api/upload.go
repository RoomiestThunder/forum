package api

import (
	"context"
	"errors"
	"io"
	"net/http"

	"forum/internal/storage"
)

// fileStore is the part of the object store the API uses. *storage.Client
// implements it; tests substitute a fake so the handlers can be exercised
// without a running MinIO.
type fileStore interface {
	Upload(ctx context.Context, r io.Reader) (objectKey, contentType, url string, size int64, err error)
	Delete(ctx context.Context, objectKey string) error
}

const (
	maxUploadBytes = 5 << 20
	// uploadBodyLimit leaves room for the multipart framing around a file of
	// maxUploadBytes. The precise size limit is enforced by the storage layer
	// as the file streams; this only stops an absurdly large body being read.
	uploadBodyLimit = maxUploadBytes + 1<<20
)

type uploadedFile struct {
	objectKey   string
	contentType string
	url         string
	filename    string
	size        int64
}

// receiveUpload reads the image in a multipart request's "file" part and stores
// it. On failure it has already written the response and returns false.
//
// Failures that are the uploader's fault answer with a 4xx that says why (415
// for the wrong kind of file, 413 for too big a one); everything else is a
// storage problem, which is logged and reported without its details - the
// error text names the storage endpoint.
func (s *Server) receiveUpload(w http.ResponseWriter, r *http.Request) (*uploadedFile, bool) {
	if s.storage == nil {
		jsonError(w, "file storage not configured", http.StatusServiceUnavailable)
		return nil, false
	}

	r.Body = http.MaxBytesReader(w, r.Body, uploadBodyLimit)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			jsonError(w, "file too large (max 5 MB)", http.StatusRequestEntityTooLarge)
		} else {
			jsonError(w, "invalid form data", http.StatusBadRequest)
		}
		return nil, false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "file field required", http.StatusBadRequest)
		return nil, false
	}
	defer func() { _ = file.Close() }()

	key, contentType, url, size, err := s.storage.Upload(r.Context(), file)
	switch {
	case errors.Is(err, storage.ErrUnsupportedType):
		jsonError(w, err.Error(), http.StatusUnsupportedMediaType)
		return nil, false
	case errors.Is(err, storage.ErrTooLarge):
		jsonError(w, err.Error(), http.StatusRequestEntityTooLarge)
		return nil, false
	case err != nil:
		s.internalError(w, r, err, "upload file")
		return nil, false
	}
	return &uploadedFile{objectKey: key, contentType: contentType, url: url, filename: header.Filename, size: size}, true
}

// discardUpload removes an object whose database record couldn't be saved, so
// a failed request doesn't leave an unreferenced file behind. Best effort: the
// failure that matters has already been reported.
func (s *Server) discardUpload(r *http.Request, f *uploadedFile) {
	if err := s.storage.Delete(r.Context(), f.objectKey); err != nil {
		s.logger.Warn("could not remove orphaned upload", "object", f.objectKey, "err", err)
	}
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	f, ok := s.receiveUpload(w, r)
	if !ok {
		return
	}
	uploadID, err := s.store.CreateUpload(userIDFromCtx(r), f.filename, f.objectKey, f.contentType, f.size, f.url)
	if err != nil {
		s.discardUpload(r, f)
		s.storeError(w, r, err, "save upload")
		return
	}
	jsonResponse(w, map[string]interface{}{"id": uploadID, "url": f.url}, http.StatusCreated)
}
