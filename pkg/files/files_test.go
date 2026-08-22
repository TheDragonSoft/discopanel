package files

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractArchive_ValidZip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "files_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	zipPath := filepath.Join(tempDir, "test.zip")
	destPath := filepath.Join(tempDir, "extracted")

	// Create a valid zip archive
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, err := zw.Create("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("hello world")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(zipPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	count, err := ExtractArchive(context.Background(), zipPath, destPath, nil)
	if err != nil {
		t.Fatalf("ExtractArchive failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 file extracted, got %d", count)
	}

	content, err := os.ReadFile(filepath.Join(destPath, "hello.txt"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", string(content))
	}
}

func TestExtractArchive_ZipSlipRejection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "files_test_slip_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	zipPath := filepath.Join(tempDir, "slip.zip")
	destPath := filepath.Join(tempDir, "extracted")

	// Create a malicious zip archive with path traversal
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, err := zw.Create("../../evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("malicious content")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(zipPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = ExtractArchive(context.Background(), zipPath, destPath, nil)
	if err == nil {
		t.Fatal("expected error extracting zipslip archive, got nil")
	}
	if !strings.Contains(err.Error(), "illegal file path in archive") {
		t.Fatalf("expected illegal file path error, got: %v", err)
	}
}

func TestCreateZipToWriter_PathSafety(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "files_test_zip_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	baseDir := filepath.Join(tempDir, "base")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "file1.txt"), []byte("data1"), 0644); err != nil {
		t.Fatal(err)
	}

	// Outside file
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	// Trying to include path traversal in paths
	_, err = CreateZipToWriter([]string{"../outside/secret.txt"}, baseDir, buf, false)
	if err == nil {
		t.Fatal("expected error archiving path outside baseDir, got nil")
	}

	// Safe zip creation
	buf.Reset()
	count, err := CreateZipToWriter([]string{"file1.txt"}, baseDir, buf, true)
	if err != nil {
		t.Fatalf("expected successful zip creation, got: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 file archived, got %d", count)
	}
}

func TestCopyDir_RecursionPrevention(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "files_test_copydir_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "doc.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test 1: dst inside src (recursive)
	dstInside := filepath.Join(srcDir, "nested_dst")
	err = CopyDir(srcDir, dstInside)
	if err == nil {
		t.Fatal("expected error copying src into itself, got nil")
	}

	// Test 2: safe copy to sibling dir
	dstSibling := filepath.Join(tempDir, "dst")
	err = CopyDir(srcDir, dstSibling)
	if err != nil {
		t.Fatalf("expected successful CopyDir, got: %v", err)
	}

	copiedData, err := os.ReadFile(filepath.Join(dstSibling, "doc.txt"))
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(copiedData) != "content" {
		t.Fatalf("expected 'content', got '%s'", string(copiedData))
	}
}

func TestIsTextFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "files_test_text_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	textFile := filepath.Join(tempDir, "text.txt")
	if err := os.WriteFile(textFile, []byte("plain text with\nlines"), 0644); err != nil {
		t.Fatal(err)
	}
	if !IsTextFile(textFile) {
		t.Fatalf("expected true for plain text file")
	}

	binFile := filepath.Join(tempDir, "binary.bin")
	if err := os.WriteFile(binFile, []byte{0x00, 0xFF, 0xFE, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	if IsTextFile(binFile) {
		t.Fatalf("expected false for binary file with null bytes")
	}
}
