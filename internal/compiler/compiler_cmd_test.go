package compiler

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/oriys/nova/internal/domain"
)

func TestDockerCreateArgs_UsesShellEntrypoint(t *testing.T) {
	got := dockerCreateArgs(
		"nova-compile-test",
		"ghcr.io/graalvm/native-image-community:21",
		"cd /work && javac *.java && native-image --static --no-fallback -o handler Main",
		domain.RuntimeGraalVM,
	)

	want := []string{
		"create",
		"--platform", "linux/" + runtime.GOARCH,
		"--network", "host",
		"--name", "nova-compile-test",
		"--entrypoint", "/bin/sh",
		"ghcr.io/graalvm/native-image-community:21",
		"-c", "cd /work && javac *.java && native-image --static --no-fallback -o handler Main",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dockerCreateArgs mismatch\nwant: %v\ngot:  %v", want, got)
	}
}

func TestResolveCompilePlatform_GraalVMOverride(t *testing.T) {
	t.Setenv("NOVA_COMPILE_PLATFORM", "linux/amd64")
	t.Setenv("NOVA_GRAALVM_COMPILE_PLATFORM", "linux/arm64")

	got := resolveCompilePlatform(domain.RuntimeGraalVM)
	if got != "linux/arm64" {
		t.Fatalf("expected graalvm override platform linux/arm64, got %s", got)
	}
}

func TestResolveCompilePlatform_Default(t *testing.T) {
	t.Setenv("NOVA_COMPILE_PLATFORM", "")
	t.Setenv("NOVA_GRAALVM_COMPILE_PLATFORM", "")

	got := resolveCompilePlatform(domain.RuntimeGo)
	want := "linux/" + runtime.GOARCH
	if got != want {
		t.Fatalf("expected default platform %s, got %s", want, got)
	}
}
