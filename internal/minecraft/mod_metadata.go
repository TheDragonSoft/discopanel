package minecraft

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	pelletiertoml "github.com/pelletier/go-toml/v2"
	yaml "gopkg.in/yaml.v3"
)

// ModMetadata holds extracted metadata and icons from a mod/plugin jar
type ModMetadata struct {
	ModID       string
	DisplayName string
	Version     string
	Description string
	Author      string
	Website     string
	IconDataURL string
}

// FabricModJSON represents the fabric.mod.json schema subset
type FabricModJSON struct {
	SchemaVersion int         `json:"schemaVersion"`
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Version       string      `json:"version"`
	Description   string      `json:"description"`
	Icon          interface{} `json:"icon"` // string or map[string]string
	Authors       interface{} `json:"authors"`
	Contact       struct {
		Homepage string `json:"homepage"`
		Sources  string `json:"sources"`
		Issues   string `json:"issues"`
	} `json:"contact"`
}

// QuiltModJSON represents the quilt.mod.json schema subset
type QuiltModJSON struct {
	QuiltLoader struct {
		ID       string `json:"id"`
		Metadata struct {
			Name         string      `json:"name"`
			Version      string      `json:"version"`
			Description  string      `json:"description"`
			Icon         interface{} `json:"icon"`
			Contributors interface{} `json:"contributors"`
			Contact      struct {
				Homepage string `json:"homepage"`
				Sources  string `json:"sources"`
				Issues   string `json:"issues"`
			} `json:"contact"`
		} `json:"metadata"`
	} `json:"quilt_loader"`
	Icon interface{} `json:"icon"`
}

// ForgeModsTOML represents META-INF/mods.toml or neoforge.mods.toml
type ForgeModsTOML struct {
	Mods []struct {
		ModID       string `toml:"modId"`
		DisplayName string `toml:"displayName"`
		Version     string `toml:"version"`
		Description string `toml:"description"`
		LogoFile    string `toml:"logoFile"`
		Authors     string `toml:"authors"`
		DisplayURL  string `toml:"displayURL"`
	} `toml:"mods"`
}

// LegacyMCModInfo represents mcmod.info (1.12 and older)
type LegacyMCModInfo struct {
	ModID       string   `json:"modid"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	LogoFile    string   `json:"logoFile"`
	URL         string   `json:"url"`
	AuthorList  []string `json:"authorList"`
}

// PluginYAML represents Bukkit / Spigot / Paper plugin.yml
type PluginYAML struct {
	Name        string      `yaml:"name"`
	Version     string      `yaml:"version"`
	Description string      `yaml:"description"`
	Author      string      `yaml:"author"`
	Authors     interface{} `yaml:"authors"`
	Website     string      `yaml:"website"`
}

// ExtractModMetadata opens a .jar file and parses its metadata and embedded icon
func ExtractModMetadata(jarPath string) (*ModMetadata, error) {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	// Default fallback display name based on filename
	baseName := filepath.Base(jarPath)
	ext := filepath.Ext(baseName)
	defaultDisplayName := strings.TrimSuffix(baseName, ext)

	meta := &ModMetadata{
		DisplayName: defaultDisplayName,
	}

	var iconPath string
	var modID string

	// 1. Try Fabric (fabric.mod.json)
	if file := findZipFile(r, "fabric.mod.json"); file != nil {
		if data, err := readZipEntry(file, 512*1024); err == nil {
			var fabric FabricModJSON
			if err := json.Unmarshal(data, &fabric); err == nil {
				modID = fabric.ID
				meta.ModID = fabric.ID
				if fabric.Name != "" {
					meta.DisplayName = fabric.Name
				}
				meta.Version = fabric.Version
				meta.Description = fabric.Description
				meta.Author = parseAuthors(fabric.Authors)
				if fabric.Contact.Homepage != "" {
					meta.Website = fabric.Contact.Homepage
				} else if fabric.Contact.Sources != "" {
					meta.Website = fabric.Contact.Sources
				}
				iconPath = parseIconPath(fabric.Icon)
			}
		}
	}

	// 2. Try Quilt (quilt.mod.json)
	if meta.ModID == "" {
		if file := findZipFile(r, "quilt.mod.json"); file != nil {
			if data, err := readZipEntry(file, 512*1024); err == nil {
				var quilt QuiltModJSON
				if err := json.Unmarshal(data, &quilt); err == nil {
					modID = quilt.QuiltLoader.ID
					meta.ModID = modID
					if quilt.QuiltLoader.Metadata.Name != "" {
						meta.DisplayName = quilt.QuiltLoader.Metadata.Name
					}
					meta.Version = quilt.QuiltLoader.Metadata.Version
					meta.Description = quilt.QuiltLoader.Metadata.Description
					meta.Author = parseAuthors(quilt.QuiltLoader.Metadata.Contributors)
					if quilt.QuiltLoader.Metadata.Contact.Homepage != "" {
						meta.Website = quilt.QuiltLoader.Metadata.Contact.Homepage
					}
					if icon := parseIconPath(quilt.QuiltLoader.Metadata.Icon); icon != "" {
						iconPath = icon
					} else {
						iconPath = parseIconPath(quilt.Icon)
					}
				}
			}
		}
	}

	// 3. Try NeoForge / Forge (neoforge.mods.toml / mods.toml)
	if meta.ModID == "" {
		candidates := []string{"META-INF/neoforge.mods.toml", "META-INF/mods.toml"}
		for _, candidate := range candidates {
			if file := findZipFile(r, candidate); file != nil {
				if data, err := readZipEntry(file, 512*1024); err == nil {
					var tomlData ForgeModsTOML
					if err := pelletiertoml.Unmarshal(data, &tomlData); err == nil && len(tomlData.Mods) > 0 {
						mod := tomlData.Mods[0]
						modID = mod.ModID
						meta.ModID = mod.ModID
						if mod.DisplayName != "" {
							meta.DisplayName = mod.DisplayName
						}
						meta.Version = mod.Version
						meta.Description = mod.Description
						meta.Author = mod.Authors
						meta.Website = mod.DisplayURL
						iconPath = mod.LogoFile
						break
					}
				}
			}
		}
	}

	// 4. Try Legacy Forge (mcmod.info)
	if meta.ModID == "" {
		if file := findZipFile(r, "mcmod.info"); file != nil {
			if data, err := readZipEntry(file, 512*1024); err == nil {
				var legacyList []LegacyMCModInfo
				if err := json.Unmarshal(data, &legacyList); err == nil && len(legacyList) > 0 {
					item := legacyList[0]
					modID = item.ModID
					meta.ModID = item.ModID
					if item.Name != "" {
						meta.DisplayName = item.Name
					}
					meta.Version = item.Version
					meta.Description = item.Description
					meta.Author = strings.Join(item.AuthorList, ", ")
					meta.Website = item.URL
					iconPath = item.LogoFile
				} else {
					var wrapper struct {
						ModList []LegacyMCModInfo `json:"modList"`
					}
					if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.ModList) > 0 {
						item := wrapper.ModList[0]
						modID = item.ModID
						meta.ModID = item.ModID
						if item.Name != "" {
							meta.DisplayName = item.Name
						}
						meta.Version = item.Version
						meta.Description = item.Description
						meta.Author = strings.Join(item.AuthorList, ", ")
						meta.Website = item.URL
						iconPath = item.LogoFile
					}
				}
			}
		}
	}

	// 5. Try Bukkit / Spigot / Paper (plugin.yml / paper-plugin.yml)
	if meta.ModID == "" {
		pluginCandidates := []string{"plugin.yml", "paper-plugin.yml"}
		for _, cand := range pluginCandidates {
			if file := findZipFile(r, cand); file != nil {
				if data, err := readZipEntry(file, 512*1024); err == nil {
					var p PluginYAML
					if err := yaml.Unmarshal(data, &p); err == nil && p.Name != "" {
						modID = p.Name
						meta.ModID = p.Name
						meta.DisplayName = p.Name
						meta.Version = p.Version
						meta.Description = p.Description
						if p.Author != "" {
							meta.Author = p.Author
						} else {
							meta.Author = parseAuthors(p.Authors)
						}
						meta.Website = p.Website
						break
					}
				}
			}
		}
	}

	// Extract the icon Data URL
	meta.IconDataURL = extractIconFromZip(&r.Reader, iconPath, modID)

	return meta, nil
}

// findZipFile searches case-insensitively for a file by exact name inside zip
func findZipFile(r *zip.ReadCloser, name string) *zip.File {
	lower := strings.ToLower(name)
	for _, f := range r.File {
		if strings.ToLower(f.Name) == lower {
			return f
		}
	}
	return nil
}

// readZipEntry reads up to maxBytes from a zip entry
func readZipEntry(f *zip.File, maxBytes int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	buf := new(bytes.Buffer)
	_, err = io.CopyN(buf, rc, maxBytes)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf.Bytes(), nil
}

// parseIconPath parses string or map icon definitions
func parseIconPath(icon interface{}) string {
	if icon == nil {
		return ""
	}
	if s, ok := icon.(string); ok {
		return s
	}
	if m, ok := icon.(map[string]interface{}); ok {
		// Prefer largest resolution or first found
		sizes := []string{"512", "256", "128", "64", "32", "16"}
		for _, size := range sizes {
			if path, ok := m[size].(string); ok && path != "" {
				return path
			}
		}
		for _, v := range m {
			if path, ok := v.(string); ok && path != "" {
				return path
			}
		}
	}
	return ""
}

// parseAuthors extracts a comma-separated author string from various formats
func parseAuthors(authors interface{}) string {
	if authors == nil {
		return ""
	}
	if s, ok := authors.(string); ok {
		return s
	}
	if list, ok := authors.([]interface{}); ok {
		names := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok && s != "" {
				names = append(names, s)
			} else if m, ok := item.(map[string]interface{}); ok {
				if name, ok := m["name"].(string); ok && name != "" {
					names = append(names, name)
				}
			}
		}
		return strings.Join(names, ", ")
	}
	if m, ok := authors.(map[string]interface{}); ok {
		names := make([]string, 0, len(m))
		for k := range m {
			names = append(names, k)
		}
		return strings.Join(names, ", ")
	}
	return ""
}

// extractIconFromZip retrieves the icon bytes and formats as a base64 data URI
func extractIconFromZip(r *zip.Reader, iconPath string, modID string) string {
	var targetFile *zip.File

	// 1. Direct path lookup
	if iconPath != "" {
		normalized := strings.TrimPrefix(iconPath, "/")
		targetFile = findZipEntryInReader(r, normalized)

		// If not found directly, try prefixing with assets/<modID>/
		if targetFile == nil && modID != "" {
			targetFile = findZipEntryInReader(r, fmt.Sprintf("assets/%s/%s", modID, normalized))
		}
		// If still not found, try assets/
		if targetFile == nil {
			targetFile = findZipEntryInReader(r, fmt.Sprintf("assets/%s", normalized))
		}
	}

	// 2. Candidate heuristic lookup
	if targetFile == nil {
		candidates := []string{}
		if modID != "" {
			candidates = append(candidates,
				fmt.Sprintf("assets/%s/icon.png", modID),
				fmt.Sprintf("assets/%s/logo.png", modID),
				fmt.Sprintf("assets/%s/icon_large.png", modID),
			)
		}
		candidates = append(candidates,
			"icon.png",
			"logo.png",
			"pack.png",
			"assets/icon.png",
		)

		for _, cand := range candidates {
			if f := findZipEntryInReader(r, cand); f != nil {
				targetFile = f
				break
			}
		}
	}

	// 3. Fallback regex lookup for any asset icon/logo png
	if targetFile == nil {
		iconRegex := regexp.MustCompile(`(?i)^assets/[^/]+/(icon|logo|pack)\.(png|jpg|jpeg|webp)$`)
		for _, f := range r.File {
			if iconRegex.MatchString(f.Name) {
				targetFile = f
				break
			}
		}
	}

	if targetFile == nil {
		return ""
	}

	// Read image bytes (limit to 1.5MB)
	data, err := readZipEntry(targetFile, 1536*1024)
	if err != nil || len(data) == 0 {
		return ""
	}

	// Determine MIME type
	ext := strings.ToLower(filepath.Ext(targetFile.Name))
	mimeType := "image/png"
	switch ext {
	case ".png":
		mimeType = "image/png"
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".webp":
		mimeType = "image/webp"
	case ".svg":
		mimeType = "image/svg+xml"
	case ".gif":
		mimeType = "image/gif"
	default:
		detected := http.DetectContentType(data)
		if strings.HasPrefix(detected, "image/") {
			mimeType = detected
		}
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
}

func findZipEntryInReader(r *zip.Reader, path string) *zip.File {
	target := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	for _, f := range r.File {
		normalized := strings.ToLower(strings.ReplaceAll(f.Name, "\\", "/"))
		if normalized == target {
			return f
		}
	}
	return nil
}
