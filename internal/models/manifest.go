package models

import (
	"errors"
	"time"
)

type Manifest struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Family       string            `json:"family"`
	Task         string            `json:"task"`
	Path         string            `json:"path"`
	Backend      string            `json:"backend"`
	Size         int64             `json:"size"`
	Format       string            `json:"format"`
	Capabilities []string          `json:"capabilities"`
	Languages    []string          `json:"languages"`
	Voices       []string          `json:"voices"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

func (m *Manifest) Validate() error {
	if m.ID == "" {
		return errors.New("manifest: id is required")
	}
	if m.Name == "" {
		return errors.New("manifest: name is required")
	}
	if m.Family == "" {
		return errors.New("manifest: family is required")
	}
	if m.Task == "" {
		return errors.New("manifest: task is required")
	}
	if m.Path == "" {
		return errors.New("manifest: path is required")
	}
	if m.Backend == "" {
		return errors.New("manifest: backend is required")
	}
	return nil
}

func (m *Manifest) SupportsCapability(cap string) bool {
	for _, c := range m.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

func (m *Manifest) SupportsLanguage(lang string) bool {
	for _, l := range m.Languages {
		if l == lang {
			return true
		}
	}
	return false
}
