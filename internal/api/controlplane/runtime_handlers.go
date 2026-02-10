package controlplane

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/oriys/nova/internal/store"
)

// ListRuntimes handles GET /runtimes
func (h *Handler) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	runtimes, err := h.Store.ListRuntimes(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if runtimes == nil {
		runtimes = []*store.RuntimeRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runtimes)
}

// CreateRuntime handles POST /runtimes
func (h *Handler) CreateRuntime(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID            string            `json:"id"`
		Name          string            `json:"name"`
		Version       string            `json:"version"`
		Status        string            `json:"status"`
		ImageName     string            `json:"image_name"`
		Entrypoint    []string          `json:"entrypoint"`
		FileExtension string            `json:"file_extension"`
		EnvVars       map[string]string `json:"env_vars"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Version == "" {
		req.Version = "dynamic"
	}
	if req.Status == "" {
		req.Status = "available"
	}
	if req.ImageName == "" {
		http.Error(w, "image_name is required", http.StatusBadRequest)
		return
	}
	if len(req.Entrypoint) == 0 {
		http.Error(w, "entrypoint is required", http.StatusBadRequest)
		return
	}
	if req.FileExtension == "" {
		http.Error(w, "file_extension is required", http.StatusBadRequest)
		return
	}

	rt := &store.RuntimeRecord{
		ID:            req.ID,
		Name:          req.Name,
		Version:       req.Version,
		Status:        req.Status,
		ImageName:     req.ImageName,
		Entrypoint:    req.Entrypoint,
		FileExtension: req.FileExtension,
		EnvVars:       req.EnvVars,
	}

	if err := h.Store.SaveRuntime(r.Context(), rt); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rt)
}

// DeleteRuntime handles DELETE /runtimes/{id}
func (h *Handler) DeleteRuntime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "runtime id is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteRuntime(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "deleted",
		"id":     id,
	})
}

// UploadRuntime handles POST /runtimes/upload (multipart/form-data)
func (h *Handler) UploadRuntime(w http.ResponseWriter, r *http.Request) {
	if h.RootfsDir == "" {
		http.Error(w, "rootfs directory not configured", http.StatusInternalServerError)
		return
	}

	// Limit request body to 2GB
	const maxUploadSize = 2 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	// Get uploaded file
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate .ext4 extension
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".ext4") {
		http.Error(w, "file must have .ext4 extension", http.StatusBadRequest)
		return
	}

	// Parse metadata JSON
	metadataStr := r.FormValue("metadata")
	if metadataStr == "" {
		http.Error(w, "metadata field is required", http.StatusBadRequest)
		return
	}

	var meta struct {
		ID            string            `json:"id"`
		Name          string            `json:"name"`
		Version       string            `json:"version"`
		Entrypoint    []string          `json:"entrypoint"`
		FileExtension string            `json:"file_extension"`
		EnvVars       map[string]string `json:"env_vars"`
	}
	if err := json.Unmarshal([]byte(metadataStr), &meta); err != nil {
		http.Error(w, "invalid metadata JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if meta.ID == "" {
		http.Error(w, "metadata.id is required", http.StatusBadRequest)
		return
	}
	if meta.Name == "" {
		http.Error(w, "metadata.name is required", http.StatusBadRequest)
		return
	}
	if len(meta.Entrypoint) == 0 {
		http.Error(w, "metadata.entrypoint is required", http.StatusBadRequest)
		return
	}
	if meta.FileExtension == "" {
		http.Error(w, "metadata.file_extension is required", http.StatusBadRequest)
		return
	}
	if meta.Version == "" {
		meta.Version = "custom"
	}

	// Sanitize runtime ID to prevent path traversal
	safeID := sanitizeFilename(meta.ID)
	if safeID == "" {
		http.Error(w, "invalid runtime id", http.StatusBadRequest)
		return
	}

	// Validate ext4 magic number (superblock at offset 1024, magic at offset 0x38 = 0xEF53)
	if !validateExt4Header(file) {
		http.Error(w, "file is not a valid ext4 image", http.StatusBadRequest)
		return
	}
	// Seek back to beginning for writing
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "failed to read file", http.StatusInternalServerError)
		return
	}

	// Check if file already exists
	imageName := safeID + ".ext4"
	destPath := filepath.Join(h.RootfsDir, imageName)
	if _, err := os.Stat(destPath); err == nil {
		http.Error(w, fmt.Sprintf("rootfs image already exists: %s", imageName), http.StatusConflict)
		return
	}

	// Ensure rootfs directory exists
	if err := os.MkdirAll(h.RootfsDir, 0755); err != nil {
		http.Error(w, "failed to create rootfs directory", http.StatusInternalServerError)
		return
	}

	// Write to temp file then atomic rename
	tmpFile, err := os.CreateTemp(h.RootfsDir, ".upload-*.ext4.tmp")
	if err != nil {
		http.Error(w, "failed to create temp file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmpFile.Name()
	defer func() {
		// Clean up temp file on any error
		os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		http.Error(w, "failed to write file", http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	// Atomic rename to destination
	if err := os.Rename(tmpPath, destPath); err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}

	// Save runtime record to database
	rt := &store.RuntimeRecord{
		ID:            safeID,
		Name:          meta.Name,
		Version:       meta.Version,
		Status:        "available",
		ImageName:     imageName,
		Entrypoint:    meta.Entrypoint,
		FileExtension: meta.FileExtension,
		EnvVars:       meta.EnvVars,
	}

	if err := h.Store.SaveRuntime(r.Context(), rt); err != nil {
		// Rollback: remove uploaded file
		os.Remove(destPath)
		http.Error(w, "failed to save runtime: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rt)
}

var safeFilenameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = safeFilenameRe.ReplaceAllString(name, "")
	return name
}

func validateExt4Header(r io.ReadSeeker) bool {
	// ext4 superblock starts at offset 1024, magic number at offset 0x38 within superblock
	buf := make([]byte, 1024+0x38+2)
	if _, err := io.ReadFull(r, buf); err != nil {
		return false
	}
	magic := binary.LittleEndian.Uint16(buf[1024+0x38:])
	return magic == 0xEF53
}
