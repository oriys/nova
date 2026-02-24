package codefile

import "testing"

func TestShouldBeExecutable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		content []byte
		want    bool
	}{
		{
			name:    "elf binary",
			path:    "handler",
			content: []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01},
			want:    true,
		},
		{
			name:    "shebang",
			path:    "main.txt",
			content: []byte("#!/bin/sh\necho ok\n"),
			want:    true,
		},
		{
			name:    "script extension",
			path:    "handler.py",
			content: []byte("print('ok')\n"),
			want:    true,
		},
		{
			name:    "handler basename",
			path:    "subdir/handler",
			content: []byte("any content"),
			want:    true,
		},
		{
			name:    "bootstrap basename",
			path:    "bootstrap",
			content: []byte("any content"),
			want:    true,
		},
		{
			name:    "plain source file",
			path:    "main.go",
			content: []byte("package main\n"),
			want:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ShouldBeExecutable(tc.path, tc.content)
			if got != tc.want {
				t.Fatalf("ShouldBeExecutable(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
