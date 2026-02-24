package controlplane

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestDetectArchiveType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{"zip", []byte{0x50, 0x4B, 0x03, 0x04, 0, 0, 0, 0}, "zip"},
		{"gzip", []byte{0x1f, 0x8b, 0, 0, 0, 0}, "tar.gz"},
		{"too_short", []byte{0x50}, ""},
		{"unknown", []byte{0, 0, 0, 0, 0, 0, 0, 0}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectArchiveType(tt.data)
			if got != tt.expected {
				t.Fatalf("DetectArchiveType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDetectArchiveType_Tar(t *testing.T) {
	// Create a valid tar header with "ustar" at offset 257
	data := make([]byte, 512)
	copy(data[257:], "ustar")
	got := DetectArchiveType(data)
	if got != "tar" {
		t.Fatalf("expected 'tar', got %q", got)
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello.py", "hello.py"},
		{"dir/file.py", "dir/file.py"},
		{"../etc/passwd", "etc/passwd"},
		{".hidden", "hidden"},
		{"dir/.hidden/file", ""},
		{"/absolute/path", "absolute/path"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizePath(tt.input)
			if got != tt.expected {
				t.Fatalf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizePath_LongPath(t *testing.T) {
	longPath := ""
	for i := 0; i < 300; i++ {
		longPath += "a"
	}
	if got := sanitizePath(longPath); got != "" {
		t.Fatal("expected empty for path longer than maxPathLength")
	}
}

func TestExtractArchive_Zip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fw, err := zw.Create("hello.py")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("print('hello')"))

	fw, err = zw.Create("lib/utils.py")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("# utils"))
	zw.Close()

	files, err := ExtractArchive(buf.Bytes(), "zip")
	if err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if string(files["hello.py"]) != "print('hello')" {
		t.Fatal("unexpected content")
	}
}

func TestExtractArchive_TarGz(t *testing.T) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	content := []byte("print('hello')")
	tw.WriteHeader(&tar.Header{
		Name:     "hello.py",
		Size:     int64(len(content)),
		Mode:     0644,
		Typeflag: tar.TypeReg,
	})
	tw.Write(content)
	tw.Close()
	gzw.Close()

	files, err := ExtractArchive(buf.Bytes(), "tar.gz")
	if err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestExtractArchive_EmptyZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zw.Close()

	_, err := ExtractArchive(buf.Bytes(), "zip")
	if err == nil {
		t.Fatal("expected error for empty archive")
	}
}

func TestExtractArchive_UnsupportedType(t *testing.T) {
	_, err := ExtractArchive([]byte("data"), "rar")
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestExtractArchive_ZipWithHiddenFiles(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fw, _ := zw.Create(".hidden")
	fw.Write([]byte("secret"))

	fw, _ = zw.Create("visible.py")
	fw.Write([]byte("code"))
	zw.Close()

	files, err := ExtractArchive(buf.Bytes(), "zip")
	if err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	// ".hidden" gets trimmed to "hidden" which is NOT hidden per the code
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestExtractArchive_ZipWithDirectories(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// Add a directory entry
	zw.Create("dir/")
	fw, _ := zw.Create("dir/file.py")
	fw.Write([]byte("code"))
	zw.Close()

	files, err := ExtractArchive(buf.Bytes(), "zip")
	if err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file (dir skipped), got %d", len(files))
	}
}
