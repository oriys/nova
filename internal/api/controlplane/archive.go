package controlplane

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const (
	maxArchiveSize     = 100 * 1024 * 1024 // 100MB max archive size
	maxFileSize        = 50 * 1024 * 1024  // 50MB max single file size
	maxFiles           = 1000              // Max files in archive
	maxPathLength      = 256               // Max file path length
)

// ExtractArchive extracts files from a zip or tar.gz archive
func ExtractArchive(data []byte, archiveType string) (map[string][]byte, error) {
	switch archiveType {
	case "zip":
		return extractZip(data)
	case "tar.gz", "tgz", "tar":
		return extractTarGz(data, archiveType == "tar")
	default:
		return nil, fmt.Errorf("unsupported archive type: %s", archiveType)
	}
}

// extractZip extracts files from a zip archive
func extractZip(data []byte) (map[string][]byte, error) {
	if len(data) > maxArchiveSize {
		return nil, fmt.Errorf("archive too large: %d bytes (max %d)", len(data), maxArchiveSize)
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	files := make(map[string][]byte)
	for _, f := range reader.File {
		if len(files) >= maxFiles {
			return nil, fmt.Errorf("too many files in archive (max %d)", maxFiles)
		}

		// Skip directories
		if f.FileInfo().IsDir() {
			continue
		}

		// Sanitize path
		path := sanitizePath(f.Name)
		if path == "" {
			continue
		}

		// Check file size
		if f.UncompressedSize64 > uint64(maxFileSize) {
			return nil, fmt.Errorf("file %s too large: %d bytes (max %d)", path, f.UncompressedSize64, maxFileSize)
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open file %s: %w", path, err)
		}

		content, err := io.ReadAll(io.LimitReader(rc, maxFileSize+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", path, err)
		}
		if len(content) > maxFileSize {
			return nil, fmt.Errorf("file %s too large: %d bytes (max %d)", path, len(content), maxFileSize)
		}

		files[path] = content
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("archive is empty")
	}

	return files, nil
}

// extractTarGz extracts files from a tar.gz or plain tar archive
func extractTarGz(data []byte, plainTar bool) (map[string][]byte, error) {
	if len(data) > maxArchiveSize {
		return nil, fmt.Errorf("archive too large: %d bytes (max %d)", len(data), maxArchiveSize)
	}

	var reader io.Reader = bytes.NewReader(data)

	if !plainTar {
		gzr, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("open gzip: %w", err)
		}
		defer gzr.Close()
		reader = gzr
	}

	tr := tar.NewReader(reader)
	files := make(map[string][]byte)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		if len(files) >= maxFiles {
			return nil, fmt.Errorf("too many files in archive (max %d)", maxFiles)
		}

		// Skip directories and non-regular files
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Sanitize path
		path := sanitizePath(header.Name)
		if path == "" {
			continue
		}

		// Check file size
		if header.Size > int64(maxFileSize) {
			return nil, fmt.Errorf("file %s too large: %d bytes (max %d)", path, header.Size, maxFileSize)
		}

		content, err := io.ReadAll(io.LimitReader(tr, maxFileSize+1))
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", path, err)
		}
		if len(content) > maxFileSize {
			return nil, fmt.Errorf("file %s too large: %d bytes (max %d)", path, len(content), maxFileSize)
		}

		files[path] = content
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("archive is empty")
	}

	return files, nil
}

// sanitizePath cleans and validates a file path from an archive
func sanitizePath(path string) string {
	// Clean the path
	path = filepath.Clean(path)

	// Remove leading slashes and dots
	path = strings.TrimLeft(path, "/.")

	// Check for path traversal
	if strings.Contains(path, "..") {
		return ""
	}

	// Check path length
	if len(path) > maxPathLength || path == "" {
		return ""
	}

	// Skip hidden files (starting with .)
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return ""
		}
	}

	return path
}

// DetectArchiveType attempts to detect the archive type from content
func DetectArchiveType(data []byte) string {
	if len(data) < 4 {
		return ""
	}

	// ZIP magic number: PK\x03\x04
	if data[0] == 0x50 && data[1] == 0x4B && data[2] == 0x03 && data[3] == 0x04 {
		return "zip"
	}

	// GZIP magic number: \x1f\x8b
	if data[0] == 0x1f && data[1] == 0x8b {
		return "tar.gz"
	}

	// TAR: check for "ustar" at offset 257
	if len(data) > 262 && string(data[257:262]) == "ustar" {
		return "tar"
	}

	return ""
}
