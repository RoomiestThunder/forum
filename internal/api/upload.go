package api

import (
	"net/http"
)

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		jsonError(w, "file storage not configured", http.StatusServiceUnavailable)
		return
	}

	// Hard cap before parsing so oversized bodies are rejected early.
	const hardLimit = 6 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, hardLimit)
	if err := r.ParseMultipartForm(5 * 1024 * 1024); err != nil {
		jsonError(w, "file too large or invalid form data", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "file field required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	userID := userIDFromCtx(r)

	// Pass -1 for size so that storage.Upload measures actual bytes instead of
	// trusting the attacker-controlled multipart header value.
	objectKey, contentType, fileURL, size, err := s.storage.Upload(
		r.Context(), file, header.Filename, -1,
	)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	uploadID, err := s.store.CreateUpload(userID, header.Filename, objectKey, contentType, size, fileURL)
	if err != nil {
		jsonError(w, "failed to save upload record", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"id":  uploadID,
		"url": fileURL,
	}, http.StatusCreated)
}
