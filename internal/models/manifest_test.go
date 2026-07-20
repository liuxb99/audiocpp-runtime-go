package models

import (
	"testing"
	"time"
)

func validManifest() *Manifest {
	return &Manifest{
		ID:           "test-model-1",
		Name:         "Test Model",
		Family:       "whisper",
		Task:         "asr",
		Path:         "/models/test.gguf",
		Backend:      "cuda",
		Size:         123456789,
		Format:       "gguf",
		Capabilities: []string{"asr", "translate"},
		Languages:    []string{"en", "zh"},
		Voices:       []string{"default"},
		Metadata:     map[string]string{"author": "test"},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

func TestManifestValidate_Valid(t *testing.T) {
	m := validManifest()
	if err := m.Validate(); err != nil {
		t.Errorf("expected valid manifest to pass, got: %v", err)
	}
}

func TestManifestValidate_MissingID(t *testing.T) {
	m := validManifest()
	m.ID = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty ID")
	} else if err.Error() != "manifest: id is required" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestManifestValidate_MissingName(t *testing.T) {
	m := validManifest()
	m.Name = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty Name")
	} else if err.Error() != "manifest: name is required" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestManifestValidate_MissingFamily(t *testing.T) {
	m := validManifest()
	m.Family = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty Family")
	} else if err.Error() != "manifest: family is required" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestManifestValidate_MissingTask(t *testing.T) {
	m := validManifest()
	m.Task = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty Task")
	} else if err.Error() != "manifest: task is required" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestManifestValidate_MissingPath(t *testing.T) {
	m := validManifest()
	m.Path = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty Path")
	} else if err.Error() != "manifest: path is required" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestManifestValidate_MissingBackend(t *testing.T) {
	m := validManifest()
	m.Backend = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty Backend")
	} else if err.Error() != "manifest: backend is required" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestManifestValidate_AllMissing(t *testing.T) {
	m := &Manifest{}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for empty manifest")
	}
	if err.Error() != "manifest: id is required" {
		t.Errorf("expected first error to be id required, got: %q", err.Error())
	}
}

func TestSupportsCapability_Match(t *testing.T) {
	m := validManifest()
	if !m.SupportsCapability("asr") {
		t.Error("expected SupportsCapability('asr') to be true")
	}
}

func TestSupportsCapability_NoMatch(t *testing.T) {
	m := validManifest()
	if m.SupportsCapability("tts") {
		t.Error("expected SupportsCapability('tts') to be false")
	}
}

func TestSupportsCapability_CaseSensitive(t *testing.T) {
	m := validManifest()
	if m.SupportsCapability("ASR") {
		t.Error("expected SupportsCapability('ASR') to be false (case sensitive)")
	}
}

func TestSupportsCapability_EmptyList(t *testing.T) {
	m := validManifest()
	m.Capabilities = nil
	if m.SupportsCapability("asr") {
		t.Error("expected SupportsCapability to be false when capabilities is nil")
	}
}

func TestSupportsLanguage_Match(t *testing.T) {
	m := validManifest()
	if !m.SupportsLanguage("en") {
		t.Error("expected SupportsLanguage('en') to be true")
	}
}

func TestSupportsLanguage_NoMatch(t *testing.T) {
	m := validManifest()
	if m.SupportsLanguage("ja") {
		t.Error("expected SupportsLanguage('ja') to be false")
	}
}

func TestSupportsLanguage_CaseSensitive(t *testing.T) {
	m := validManifest()
	if m.SupportsLanguage("EN") {
		t.Error("expected SupportsLanguage('EN') to be false (case sensitive)")
	}
}

func TestSupportsLanguage_EmptyList(t *testing.T) {
	m := validManifest()
	m.Languages = nil
	if m.SupportsLanguage("en") {
		t.Error("expected SupportsLanguage to be false when languages is nil")
	}
}
