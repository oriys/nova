package docker

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestRuntimeDockerImages_UniqueAndSorted(t *testing.T) {
	images := runtimeDockerImages("nova-runtime")
	if !sort.StringsAreSorted(images) {
		t.Fatalf("images should be sorted: %v", images)
	}

	want := []string{
		"nova-runtime-base",
		"nova-runtime-bun",
		"nova-runtime-deno",
		"nova-runtime-java",
		"nova-runtime-lua",
		"nova-runtime-node",
		"nova-runtime-php",
		"nova-runtime-python",
		"nova-runtime-ruby",
		"nova-runtime-wasm",
	}
	if !reflect.DeepEqual(images, want) {
		t.Fatalf("runtimeDockerImages mismatch\nwant: %v\ngot:  %v", want, images)
	}
}

func TestEnsureDockerImages_PullsMissingOnly(t *testing.T) {
	inspectCalls := make([]string, 0, 2)
	pullCalls := make([]string, 0, 1)

	err := ensureDockerImages(
		context.Background(),
		[]string{"img-a", "img-b"},
		func(_ context.Context, image string) (bool, error) {
			inspectCalls = append(inspectCalls, image)
			return image == "img-a", nil
		},
		func(_ context.Context, image string) error {
			pullCalls = append(pullCalls, image)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ensureDockerImages returned error: %v", err)
	}

	if !reflect.DeepEqual(inspectCalls, []string{"img-a", "img-b"}) {
		t.Fatalf("unexpected inspect calls: %v", inspectCalls)
	}
	if !reflect.DeepEqual(pullCalls, []string{"img-b"}) {
		t.Fatalf("unexpected pull calls: %v", pullCalls)
	}
}

func TestEnsureDockerImages_InspectError(t *testing.T) {
	wantErr := errors.New("inspect failed")
	err := ensureDockerImages(
		context.Background(),
		[]string{"img-a"},
		func(_ context.Context, _ string) (bool, error) {
			return false, wantErr
		},
		func(_ context.Context, _ string) error {
			return nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected inspect error, got: %v", err)
	}
}

func TestEnsureDockerImages_PullError(t *testing.T) {
	wantErr := errors.New("pull failed")
	err := ensureDockerImages(
		context.Background(),
		[]string{"img-a"},
		func(_ context.Context, _ string) (bool, error) {
			return false, nil
		},
		func(_ context.Context, _ string) error {
			return wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected pull error, got: %v", err)
	}
}
