package models

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
)

type Registry struct {
	mu       sync.RWMutex
	models   map[string]*Manifest
	filePath string
}

func NewRegistry(filePath string) *Registry {
	return &Registry{
		models:   make(map[string]*Manifest),
		filePath: filePath,
	}
}

func (r *Registry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			r.models = make(map[string]*Manifest)
			return nil
		}
		return fmt.Errorf("read registry: %w", err)
	}

	var list []*Manifest
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("unmarshal registry: %w", err)
	}

	r.models = make(map[string]*Manifest, len(list))
	for _, m := range list {
		r.models[m.ID] = m
	}
	return nil
}

func (r *Registry) Save() error {
	r.mu.RLock()
	list := make([]*Manifest, 0, len(r.models))
	for _, m := range r.models {
		list = append(list, m)
	}
	r.mu.RUnlock()

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(r.filePath), 0755); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}

	if err := os.WriteFile(r.filePath, data, 0644); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return nil
}

func (r *Registry) Get(id string) (*Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.models[id]
	return m, ok
}

func (r *Registry) List() []*Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Manifest, 0, len(r.models))
	for _, m := range r.models {
		result = append(result, m)
	}
	return result
}

func (r *Registry) ListByCapability(cap string) []*Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*Manifest
	for _, m := range r.models {
		if m.SupportsCapability(cap) {
			result = append(result, m)
		}
	}
	return result
}

func (r *Registry) ListByFamily(family string) []*Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*Manifest
	for _, m := range r.models {
		if m.Family == family {
			result = append(result, m)
		}
	}
	return result
}

func (r *Registry) Add(m *Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	r.models[m.ID] = m
	return nil
}

func (r *Registry) Remove(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.models[id]; ok {
		delete(r.models, id)
		return true
	}
	return false
}

func (r *Registry) Refresh(ctx context.Context, client *audiocpp.Client) error {
	resp, err := client.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("list models from server: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	remote := make(map[string]bool, len(resp.Data))
	now := time.Now().UTC()

	for _, mi := range resp.Data {
		remote[mi.ID] = true
		if existing, ok := r.models[mi.ID]; ok {
			existing.Family = mi.Family
			existing.Task = mi.Task
			existing.UpdatedAt = now
			if len(existing.Capabilities) == 0 {
				for _, cap := range audiocpp.TaskToCapabilities(mi.Task) {
					existing.Capabilities = append(existing.Capabilities, string(cap))
				}
			}
		} else {
			m := &Manifest{
				ID:        mi.ID,
				Name:      mi.ID,
				Family:    mi.Family,
				Task:      mi.Task,
				CreatedAt: now,
				UpdatedAt: now,
			}
			for _, cap := range audiocpp.TaskToCapabilities(mi.Task) {
				m.Capabilities = append(m.Capabilities, string(cap))
			}
			r.models[mi.ID] = m
		}
	}

	for id := range r.models {
		if !remote[id] {
			delete(r.models, id)
		}
	}

	return nil
}
