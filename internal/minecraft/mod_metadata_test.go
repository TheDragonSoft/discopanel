package minecraft

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractModMetadata_Fabric(t *testing.T) {
	// Create synthetic fabric jar
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	fabricJSON := `{
		"schemaVersion": 1,
		"id": "test-mod",
		"name": "Test Mod",
		"version": "1.0.0",
		"description": "A test fabric mod",
		"icon": "assets/test-mod/icon.png",
		"authors": ["Author One", {"name": "Author Two"}],
		"contact": {"homepage": "https://example.com"}
	}`

	fw, err := zw.Create("fabric.mod.json")
	if err != nil {
		t.Fatalf("failed to create fabric.mod.json: %v", err)
	}
	fw.Write([]byte(fabricJSON))

	iw, err := zw.Create("assets/test-mod/icon.png")
	if err != nil {
		t.Fatalf("failed to create icon: %v", err)
	}
	// Write dummy PNG header bytes
	dummyPNG := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	iw.Write(dummyPNG)

	zw.Close()

	// Write to temp file
	tmpDir := t.TempDir()
	jarPath := filepath.Join(tmpDir, "test-mod-1.0.0.jar")
	if err := os.WriteFile(jarPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write test jar: %v", err)
	}

	meta, err := ExtractModMetadata(jarPath)
	if err != nil {
		t.Fatalf("ExtractModMetadata failed: %v", err)
	}

	if meta.DisplayName != "Test Mod" {
		t.Errorf("expected DisplayName 'Test Mod', got '%s'", meta.DisplayName)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got '%s'", meta.Version)
	}
	if meta.Description != "A test fabric mod" {
		t.Errorf("expected Description 'A test fabric mod', got '%s'", meta.Description)
	}
	if !strings.Contains(meta.Author, "Author One") || !strings.Contains(meta.Author, "Author Two") {
		t.Errorf("expected authors to contain Author One and Author Two, got '%s'", meta.Author)
	}
	if meta.Website != "https://example.com" {
		t.Errorf("expected Website 'https://example.com', got '%s'", meta.Website)
	}
	if !strings.HasPrefix(meta.IconDataURL, "data:image/png;base64,") {
		t.Errorf("expected IconDataURL with PNG data URL prefix, got '%s'", meta.IconDataURL)
	}
}

func TestExtractModMetadata_RealJars(t *testing.T) {
	// If test server directory exists, test against actual files
	testModsDir := filepath.Join("..", "..", "data", "servers", "test_04b51cf5-22ea-4a5b-8676-c1636668c351", "mods")
	if entries, err := os.ReadDir(testModsDir); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".jar") {
				jarPath := filepath.Join(testModsDir, entry.Name())
				meta, err := ExtractModMetadata(jarPath)
				if err != nil {
					t.Logf("mod %s error: %v", entry.Name(), err)
					continue
				}
				t.Logf("Mod %s parsed: Name=%s, Version=%s, HasIcon=%v",
					entry.Name(), meta.DisplayName, meta.Version, meta.IconDataURL != "")
				if meta.IconDataURL == "" {
					t.Errorf("expected mod %s to have an icon", entry.Name())
				}
			}
		}
	}
}
