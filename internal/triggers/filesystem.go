package triggers

import (
"context"
"encoding/json"
"fmt"
"os"
"path/filepath"
"sync"
"time"

"github.com/oriys/nova/internal/logging"
)

// FilesystemConnector watches filesystem paths for changes
type FilesystemConnector struct {
trigger      *Trigger
handler      EventHandler
watchPath    string
pollInterval time.Duration
mu           sync.Mutex
running      bool
stopCh       chan struct{}
}

// FilesystemConfig defines filesystem trigger configuration
type FilesystemConfig struct {
Path         string `json:"path"`          // Path to watch
Pattern      string `json:"pattern"`       // File pattern (*.txt, etc.)
PollInterval int    `json:"poll_interval"` // Poll interval in seconds (default 60)
}

// NewFilesystemConnector creates a new filesystem event connector
func NewFilesystemConnector(trigger *Trigger, handler EventHandler) (*FilesystemConnector, error) {
var config FilesystemConfig
configBytes, _ := json.Marshal(trigger.Config)
if err := json.Unmarshal(configBytes, &config); err != nil {
return nil, fmt.Errorf("invalid filesystem config: %w", err)
}

if config.Path == "" {
return nil, fmt.Errorf("path is required")
}

if config.PollInterval <= 0 {
config.PollInterval = 60
}

return &FilesystemConnector{
trigger:      trigger,
handler:      handler,
watchPath:    config.Path,
pollInterval: time.Duration(config.PollInterval) * time.Second,
stopCh:       make(chan struct{}),
}, nil
}

// Start begins watching the filesystem path
func (fc *FilesystemConnector) Start(ctx context.Context) error {
fc.mu.Lock()
if fc.running {
fc.mu.Unlock()
return fmt.Errorf("connector already running")
}
fc.running = true
fc.mu.Unlock()

go fc.pollLoop(ctx)
logging.Op().Info("filesystem connector started", "trigger", fc.trigger.Name, "path", fc.watchPath)
return nil
}

// Stop stops the filesystem watcher
func (fc *FilesystemConnector) Stop() error {
fc.mu.Lock()
defer fc.mu.Unlock()

if !fc.running {
return nil
}

close(fc.stopCh)
fc.running = false
logging.Op().Info("filesystem connector stopped", "trigger", fc.trigger.Name)
return nil
}

// Type returns the trigger type
func (fc *FilesystemConnector) Type() TriggerType {
return TriggerTypeFilesystem
}

// IsHealthy checks if the connector is operational
func (fc *FilesystemConnector) IsHealthy() bool {
fc.mu.Lock()
defer fc.mu.Unlock()
return fc.running
}

func (fc *FilesystemConnector) pollLoop(ctx context.Context) {
ticker := time.NewTicker(fc.pollInterval)
defer ticker.Stop()

lastCheck := make(map[string]time.Time)

for {
select {
case <-ctx.Done():
return
case <-fc.stopCh:
return
case <-ticker.C:
fc.checkPath(ctx, lastCheck)
}
}
}

func (fc *FilesystemConnector) checkPath(ctx context.Context, lastCheck map[string]time.Time) {
files, err := filepath.Glob(fc.watchPath)
if err != nil {
logging.Op().Error("failed to glob path", "path", fc.watchPath, "error", err)
return
}

for _, file := range files {
info, err := os.Stat(file)
if err != nil {
continue
}

if info.IsDir() {
continue
}

modTime := info.ModTime()
lastMod, exists := lastCheck[file]

if !exists || modTime.After(lastMod) {
fc.handleFileEvent(ctx, file, info)
lastCheck[file] = modTime
}
}
}

func (fc *FilesystemConnector) handleFileEvent(ctx context.Context, path string, info os.FileInfo) {
event := &TriggerEvent{
TriggerID: fc.trigger.ID,
EventID:   fmt.Sprintf("%s-%d", path, info.ModTime().Unix()),
Source:    "filesystem",
Type:      "file.modified",
Data:      json.RawMessage(fmt.Sprintf(`{"path":"%s","size":%d,"mod_time":"%s"}`, path, info.Size(), info.ModTime().Format(time.RFC3339))),
Metadata: map[string]interface{}{
"path":     path,
"size":     info.Size(),
"mod_time": info.ModTime(),
},
Timestamp: time.Now(),
}

if err := fc.handler.Handle(ctx, event); err != nil {
logging.Op().Error("failed to handle file event", "path", path, "error", err)
}
}
