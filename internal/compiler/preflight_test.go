package compiler

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestCompilerDockerImages_UniqueAndSorted(t *testing.T) {
	images := compilerDockerImages()
	if !sort.StringsAreSorted(images) {
		t.Fatalf("images should be sorted: %v", images)
	}

	want := []string{
		"composer:2",
		"eclipse-temurin:21-jdk",
		"euantorano/zig:0.13.0",
		"gcc:14",
		"golang:1.23-alpine",
		"node:20-slim",
		"python:3.12-slim",
		"ruby:3.3-slim",
		"rust:1.84-alpine",
		"swift:5.10",
	}
	if !reflect.DeepEqual(images, want) {
		t.Fatalf("compilerDockerImages mismatch\nwant: %v\ngot:  %v", want, images)
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
