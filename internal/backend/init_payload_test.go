package backend

import (
	"encoding/json"
	"testing"

	"github.com/oriys/nova/internal/domain"
)

func TestBuildInitPayloadIncludesLayerAndVolumeMounts(t *testing.T) {
	fn := &domain.Function{
		Runtime:          domain.RuntimePython,
		Handler:          "main.handler",
		EnvVars:          map[string]string{"FOO": "bar"},
		RuntimeCommand:   []string{"python3", "/code/handler"},
		RuntimeExtension: ".py",
		Mode:             domain.ModeProcess,
		Name:             "hello",
		Version:          7,
		MemoryMB:         256,
		TimeoutS:         30,
		LayerPaths:       []string{"/layers/a.ext4", "/layers/b.ext4"},
		ResolvedMounts: []domain.ResolvedMount{
			{
				ImagePath: "/volumes/data.ext4",
				MountPath: "/mnt/data",
				ReadOnly:  true,
			},
		},
	}

	payload := BuildInitPayload(fn)

	if payload.LayerCount != 2 {
		t.Fatalf("expected layer_count=2, got %d", payload.LayerCount)
	}
	if len(payload.VolumeMounts) != 1 {
		t.Fatalf("expected 1 volume mount, got %d", len(payload.VolumeMounts))
	}
	if payload.VolumeMounts[0].MountPath != "/mnt/data" {
		t.Fatalf("expected mount_path=/mnt/data, got %s", payload.VolumeMounts[0].MountPath)
	}
	if !payload.VolumeMounts[0].ReadOnly {
		t.Fatalf("expected read_only=true")
	}
}

func TestMarshalInitPayloadProducesJSON(t *testing.T) {
	fn := &domain.Function{
		Runtime: domain.RuntimeNode,
		Handler: "index.handler",
		LayerPaths: []string{
			"/layers/node.ext4",
		},
	}

	raw, err := MarshalInitPayload(fn)
	if err != nil {
		t.Fatalf("MarshalInitPayload returned error: %v", err)
	}

	var got InitPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal marshaled payload: %v", err)
	}
	if got.Runtime != string(domain.RuntimeNode) {
		t.Fatalf("expected runtime=%s, got %s", domain.RuntimeNode, got.Runtime)
	}
	if got.LayerCount != 1 {
		t.Fatalf("expected layer_count=1, got %d", got.LayerCount)
	}
}
