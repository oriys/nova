package controlplane

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DockerImageInfo describes a Docker image available on the host.
type DockerImageInfo struct {
	Runtime    string `json:"runtime"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
}

// RootfsImageInfo describes a rootfs ext4 file on disk.
type RootfsImageInfo struct {
	Runtime  string `json:"runtime"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// SystemImagesResponse is the response for GET /system/images.
type SystemImagesResponse struct {
	DockerImages []DockerImageInfo `json:"docker_images"`
	RootfsImages []RootfsImageInfo `json:"rootfs_images"`
}

// ListSystemImages handles GET /system/images.
// It detects available Docker runtime images and rootfs ext4 files.
func (h *Handler) ListSystemImages(w http.ResponseWriter, r *http.Request) {
	resp := SystemImagesResponse{
		DockerImages: listDockerRuntimeImages(),
		RootfsImages: listRootfsImages(h.RootfsDir),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func listDockerRuntimeImages() []DockerImageInfo {
	out, err := exec.Command(
		"docker", "images",
		"--format", "{{.Repository}}\t{{.Tag}}\t{{.Size}}",
		"--filter", "reference=nova-runtime-*",
	).Output()
	if err != nil {
		return []DockerImageInfo{}
	}

	var images []DockerImageInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		repo := parts[0]
		runtime := strings.TrimPrefix(repo, "nova-runtime-")
		images = append(images, DockerImageInfo{
			Runtime:    runtime,
			Repository: repo,
			Tag:        parts[1],
			Size:       parts[2],
		})
	}
	return images
}

func listRootfsImages(rootfsDir string) []RootfsImageInfo {
	if rootfsDir == "" {
		return []RootfsImageInfo{}
	}

	entries, err := os.ReadDir(rootfsDir)
	if err != nil {
		return []RootfsImageInfo{}
	}

	var images []RootfsImageInfo
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".ext4") {
			continue
		}
		runtime := strings.TrimSuffix(name, ".ext4")
		// Also strip architecture suffix if present
		runtime = strings.TrimSuffix(runtime, "-amd64")
		runtime = strings.TrimSuffix(runtime, "-arm64")
		var size int64
		if info, err := entry.Info(); err == nil {
			size = info.Size()
		}
		images = append(images, RootfsImageInfo{
			Runtime:  runtime,
			Filename: name,
			Size:     size,
		})
	}

	// If main rootfs dir had nothing, also try assets/rootfs relative to working dir
	if len(images) == 0 {
		altDir := filepath.Join("assets", "rootfs")
		if altDir != rootfsDir {
			altEntries, err := os.ReadDir(altDir)
			if err == nil {
				for _, entry := range altEntries {
					name := entry.Name()
					if entry.IsDir() || !strings.HasSuffix(name, ".ext4") {
						continue
					}
					runtime := strings.TrimSuffix(name, ".ext4")
					runtime = strings.TrimSuffix(runtime, "-amd64")
					runtime = strings.TrimSuffix(runtime, "-arm64")
					var size int64
					if info, err := entry.Info(); err == nil {
						size = info.Size()
					}
					images = append(images, RootfsImageInfo{
						Runtime:  runtime,
						Filename: name,
						Size:     size,
					})
				}
			}
		}
	}
	return images
}
