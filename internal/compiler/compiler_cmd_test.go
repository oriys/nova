package compiler

import (
	"reflect"
	"testing"
)

func TestDockerCreateArgs_UsesShellEntrypoint(t *testing.T) {
	got := dockerCreateArgs(
		"nova-compile-test",
		"ghcr.io/graalvm/native-image-community:21",
		"cd /work && javac *.java && native-image --static --no-fallback -o handler Main",
	)

	want := []string{
		"create",
		"--platform", "linux/amd64",
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
