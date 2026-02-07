package layer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// Manager handles building and managing shared dependency layer images
type Manager struct {
	store      *store.Store
	storageDir string
	maxPerFunc int
}

// New creates a new layer Manager
func New(s *store.Store, storageDir string, maxPerFunc int) *Manager {
	if maxPerFunc <= 0 {
		maxPerFunc = 6
	}
	if storageDir == "" {
		storageDir = "/opt/nova/layers"
	}
	os.MkdirAll(storageDir, 0755)
	return &Manager{
		store:      s,
		storageDir: storageDir,
		maxPerFunc: maxPerFunc,
	}
}

// BuildLayer creates a new ext4 image from the provided files
func (m *Manager) BuildLayer(ctx context.Context, name string, runtime domain.Runtime, files map[string][]byte) (*domain.Layer, error) {
	id := uuid.New().String()[:12]
	imagePath := filepath.Join(m.storageDir, id+".ext4")

	// Calculate size
	var totalSize int64
	for _, content := range files {
		totalSize += int64(len(content))
	}
	sizeMB := int(float64(totalSize)/(1024*1024)*1.5) + 4 // 50% headroom + metadata
	if sizeMB < 4 {
		sizeMB = 4
	}
	if sizeMB > 512 {
		sizeMB = 512
	}

	// Create ext4 image
	f, err := os.Create(imagePath)
	if err != nil {
		return nil, fmt.Errorf("create layer image: %w", err)
	}
	if err := f.Truncate(int64(sizeMB) * 1024 * 1024); err != nil {
		f.Close()
		os.Remove(imagePath)
		return nil, fmt.Errorf("truncate layer image: %w", err)
	}
	f.Close()

	if out, err := exec.Command("mkfs.ext4", "-F", "-q", imagePath).CombinedOutput(); err != nil {
		os.Remove(imagePath)
		return nil, fmt.Errorf("mkfs.ext4: %s: %w", out, err)
	}

	// Inject files using debugfs
	tmpDir, err := os.MkdirTemp("", "nova-layer-*")
	if err != nil {
		os.Remove(imagePath)
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Collect directories
	dirs := make(map[string]bool)
	fileNames := make([]string, 0, len(files))
	for path := range files {
		fileNames = append(fileNames, path)
		parts := strings.Split(path, "/")
		for i := 1; i < len(parts); i++ {
			dir := strings.Join(parts[:i], "/")
			if dir != "" {
				dirs[dir] = true
			}
		}
	}

	var debugfsCmds strings.Builder
	// Create directories (sorted by depth)
	sortedDirs := make([]string, 0, len(dirs))
	for dir := range dirs {
		sortedDirs = append(sortedDirs, dir)
	}
	for i := range sortedDirs {
		for j := i + 1; j < len(sortedDirs); j++ {
			iDepth := strings.Count(sortedDirs[i], "/")
			jDepth := strings.Count(sortedDirs[j], "/")
			if iDepth > jDepth || (iDepth == jDepth && sortedDirs[i] > sortedDirs[j]) {
				sortedDirs[i], sortedDirs[j] = sortedDirs[j], sortedDirs[i]
			}
		}
	}
	for _, dir := range sortedDirs {
		debugfsCmds.WriteString(fmt.Sprintf("mkdir %s\n", dir))
	}

	// Write files
	for path, content := range files {
		tmpFile := filepath.Join(tmpDir, strings.ReplaceAll(path, "/", "_"))
		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			os.Remove(imagePath)
			return nil, fmt.Errorf("write temp file %s: %w", path, err)
		}
		debugfsCmds.WriteString(fmt.Sprintf("write %s %s\n", tmpFile, path))
	}

	cmd := exec.Command("debugfs", "-w", imagePath)
	cmd.Stdin = strings.NewReader(debugfsCmds.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(imagePath)
		return nil, fmt.Errorf("debugfs inject: %s: %w", out, err)
	}

	layer := &domain.Layer{
		ID:        id,
		Name:      name,
		Runtime:   runtime,
		Version:   "1.0",
		SizeMB:    sizeMB,
		Files:     fileNames,
		ImagePath: imagePath,
	}

	if err := m.store.SaveLayer(ctx, layer); err != nil {
		os.Remove(imagePath)
		return nil, fmt.Errorf("save layer: %w", err)
	}

	logging.Op().Info("layer built", "id", id, "name", name, "size_mb", sizeMB, "files", len(files))
	return layer, nil
}

// DeleteLayer removes a layer if no functions reference it
func (m *Manager) DeleteLayer(ctx context.Context, id string) error {
	// Check if any functions use this layer
	funcIDs, err := m.store.ListFunctionsByLayer(ctx, id)
	if err != nil {
		return fmt.Errorf("check layer references: %w", err)
	}
	if len(funcIDs) > 0 {
		return fmt.Errorf("layer is referenced by %d functions", len(funcIDs))
	}

	layer, err := m.store.GetLayer(ctx, id)
	if err != nil {
		return err
	}

	// Remove ext4 image
	if layer.ImagePath != "" {
		os.Remove(layer.ImagePath)
	}

	return m.store.DeleteLayer(ctx, id)
}

// ValidateFunctionLayers checks layer count and runtime compatibility
func (m *Manager) ValidateFunctionLayers(ctx context.Context, funcID string, layerIDs []string, fnRuntime domain.Runtime) error {
	if len(layerIDs) > m.maxPerFunc {
		return fmt.Errorf("too many layers: %d (max %d)", len(layerIDs), m.maxPerFunc)
	}

	for _, lid := range layerIDs {
		layer, err := m.store.GetLayer(ctx, lid)
		if err != nil {
			return fmt.Errorf("layer %s not found: %w", lid, err)
		}
		// Check runtime compatibility - layer runtime should match or be generic
		if layer.Runtime != "" && layer.Runtime != fnRuntime {
			return fmt.Errorf("layer %s (runtime %s) incompatible with function runtime %s",
				layer.Name, layer.Runtime, fnRuntime)
		}
	}
	return nil
}

// StorageDir returns the storage directory path
func (m *Manager) StorageDir() string {
	return m.storageDir
}
