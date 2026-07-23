package backend

import (
	"fmt"
	"sort"
	"sync"
)

// BackendBuilder 建立 Backend 實例的工廠函數
type BackendBuilder func() Backend

// Registry 後端註冊表（執行緒安全）
type Registry struct {
	mu       sync.RWMutex
	builders map[string]BackendBuilder
}

// NewRegistry 建立新的 Registry
func NewRegistry() *Registry {
	return &Registry{
		builders: make(map[string]BackendBuilder),
	}
}

// Register 註冊後端，若 id 已存在回傳 ErrAlreadyRegistered
func (r *Registry) Register(id string, builder BackendBuilder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.builders[id]; exists {
		return ErrAlreadyRegistered
	}
	r.builders[id] = builder
	return nil
}

// Create 建立後端實例，若 id 未註冊回傳 ErrBackendNotFound
func (r *Registry) Create(id string) (Backend, error) {
	r.mu.RLock()
	builder, exists := r.builders[id]
	r.mu.RUnlock()
	if !exists {
		return nil, ErrBackendNotFound
	}
	return builder(), nil
}

// Has 檢查後端是否已註冊
func (r *Registry) Has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.builders[id]
	return exists
}

// Names 回傳所有已註冊後端名稱（排序）
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.builders))
	for name := range r.builders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MustRegister 註冊後端，若重複則 panic（用於 init() 或 test setup）
func (r *Registry) MustRegister(id string, builder BackendBuilder) {
	if err := r.Register(id, builder); err != nil {
		panic(fmt.Sprintf("backend: failed to register %q: %v", id, err))
	}
}

// DefaultRegistry 全域預設 Registry
var DefaultRegistry = NewRegistry()
