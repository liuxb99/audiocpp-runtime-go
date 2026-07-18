package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type AnnotatedEntry struct {
	ModelEntry
	AbsPath      string   `json:"abs_path"`
	Installed    bool     `json:"installed"`
	Incomplete   bool     `json:"incomplete"`
	MissingFiles []string `json:"missing_files,omitempty"`
	Label        string   `json:"label"`
}

func LoadCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, err
	}
	return &cat, nil
}

func (c *Catalog) Annotate(bundleRoot string) []AnnotatedEntry {
	var out []AnnotatedEntry
	for _, m := range c.Models {
		rel := m.Path
		ap := rel
		if !filepath.IsAbs(rel) {
			ap = filepath.Join(bundleRoot, rel)
		}
		ap = filepath.Clean(ap)
		entry := AnnotatedEntry{
			ModelEntry: m,
			AbsPath:    ap,
			Installed:  dirExists(ap),
		}
		entry.Label = m.DisplayName
		if entry.Label == "" {
			entry.Label = m.ID
		}
		out = append(out, entry)
	}
	return out
}

func (e *AnnotatedEntry) DisplayNameLabel(language string) string {
	if language == "en" && e.DisplayNameEn != "" {
		return e.DisplayNameEn
	}
	if e.ModelEntry.DisplayName != "" {
		return e.ModelEntry.DisplayName
	}
	return e.ID
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isAbs(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/") ||
		(len(path) > 1 && path[1] == ':')
}

func (c *Catalog) EntryByID(id string) *ModelEntry {
	for i := range c.Models {
		if c.Models[i].ID == id {
			return &c.Models[i]
		}
	}
	return nil
}
