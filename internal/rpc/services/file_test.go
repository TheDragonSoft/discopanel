package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSafePath_ContainmentAndTraversal(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "file_safe_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	baseDir := filepath.Join(tempDir, "server_data")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Valid subpaths
	if !isSafePath(baseDir, filepath.Join(baseDir, "world", "level.dat")) {
		t.Error("expected true for nested valid path")
	}
	if !isSafePath(baseDir, baseDir) {
		t.Error("expected true for baseDir itself")
	}

	// 2. Traversal breakouts
	if isSafePath(baseDir, filepath.Join(baseDir, "..", "secret.txt")) {
		t.Error("expected false for parent traversal breakout")
	}
	if isSafePath(baseDir, filepath.Join(tempDir, "other_dir")) {
		t.Error("expected false for outside path")
	}

	// 3. Sibling prefix breakout
	siblingDir := baseDir + "_sibling"
	if isSafePath(baseDir, siblingDir) {
		t.Error("expected false for sibling directory prefix breakout")
	}
}

func TestIsValidFilename(t *testing.T) {
	// Valid filenames
	valid := []string{
		"server.properties",
		"world",
		"paper-1.21.1.jar",
		"ops.json",
		"user_data-2026.log",
	}
	for _, name := range valid {
		if !isValidFilename(name) {
			t.Errorf("expected %s to be valid filename", name)
		}
	}

	// Invalid filenames (traversals, slashes, reserved Windows names, trailing dots/spaces)
	invalid := []string{
		"..",
		".",
		"dir/file.txt",
		"dir\\file.txt",
		"CON",
		"con.txt",
		"PRN",
		"AUX.json",
		"NUL",
		"COM1",
		"com9.log",
		"LPT1",
		"file.txt ",
		"file.txt.",
		"",
	}
	for _, name := range invalid {
		if isValidFilename(name) {
			t.Errorf("expected %s to be invalid filename", name)
		}
	}
}
