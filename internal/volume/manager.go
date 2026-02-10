package volume

import (
"context"
"fmt"
"os"
"os/exec"
"path/filepath"

"github.com/oriys/nova/internal/domain"
"github.com/oriys/nova/internal/logging"
"github.com/oriys/nova/internal/store"
)

// Manager handles persistent volume lifecycle operations
type Manager struct {
store     *store.Store
volumeDir string
}

// Config holds volume manager configuration
type Config struct {
VolumeDir string
}

// DefaultConfig returns default volume manager configuration
func DefaultConfig() *Config {
volumeDir := os.Getenv("NOVA_VOLUME_DIR")
if volumeDir == "" {
volumeDir = "/opt/nova/volumes"
}
return &Config{
VolumeDir: volumeDir,
}
}

// NewManager creates a new volume manager
func NewManager(s *store.Store, cfg *Config) (*Manager, error) {
if cfg == nil {
cfg = DefaultConfig()
}

if err := os.MkdirAll(cfg.VolumeDir, 0755); err != nil {
return nil, fmt.Errorf("create volume directory: %w", err)
}

return &Manager{
store:     s,
volumeDir: cfg.VolumeDir,
}, nil
}

// CreateVolume creates a new persistent volume with an ext4 filesystem
func (m *Manager) CreateVolume(ctx context.Context, vol *domain.Volume) error {
imageName := fmt.Sprintf("%s-%s.ext4", vol.TenantID, vol.Name)
imagePath := filepath.Join(m.volumeDir, imageName)

if _, err := os.Stat(imagePath); err == nil {
return fmt.Errorf("volume image already exists: %s", imagePath)
}

if err := m.createExt4Image(imagePath, vol.SizeMB); err != nil {
return fmt.Errorf("create ext4 image: %w", err)
}

vol.ImagePath = imagePath

if err := m.store.CreateVolume(ctx, vol); err != nil {
os.Remove(imagePath)
return fmt.Errorf("save volume metadata: %w", err)
}

logging.Op().Info("volume created", "name", vol.Name, "size_mb", vol.SizeMB, "path", imagePath)
return nil
}

func (m *Manager) createExt4Image(path string, sizeMB int) error {
f, err := os.Create(path)
if err != nil {
return fmt.Errorf("create file: %w", err)
}

if err := f.Truncate(int64(sizeMB) * 1024 * 1024); err != nil {
f.Close()
os.Remove(path)
return fmt.Errorf("truncate file: %w", err)
}
f.Close()

cmd := exec.Command("mkfs.ext4", "-F", "-q", path)
if output, err := cmd.CombinedOutput(); err != nil {
os.Remove(path)
return fmt.Errorf("mkfs.ext4 failed: %w, output: %s", err, output)
}

return nil
}

// DeleteVolume deletes a volume and its image file
func (m *Manager) DeleteVolume(ctx context.Context, volumeID string) error {
vol, err := m.store.GetVolume(ctx, volumeID)
if err != nil {
return fmt.Errorf("get volume: %w", err)
}

if err := m.store.DeleteVolume(ctx, volumeID); err != nil {
return fmt.Errorf("delete volume metadata: %w", err)
}

if err := os.Remove(vol.ImagePath); err != nil && !os.IsNotExist(err) {
logging.Op().Warn("failed to remove volume image", "path", vol.ImagePath, "error", err)
}

logging.Op().Info("volume deleted", "id", volumeID, "name", vol.Name)
return nil
}
