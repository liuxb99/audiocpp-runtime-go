package execution

import (
	"context"
	"testing"

	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
	befake "github.com/liuxb99/audiocpp-runtime-go/internal/backend/fake"
)

func TestGate_SupportedCapability_PASS(t *testing.T) {
	be := befake.NewWithID("test-backend")
	ctx := context.Background()
	be.Start(ctx, backend.StartConfig{})

	reg := backend.NewRegistry()
	reg.MustRegister("test-backend", func() backend.Backend { return be })
	mgr := backend.NewManager(reg)
	mgr.Select("test-backend")

	gate := NewDefaultGate(mgr)
	err := gate.Check(ctx, backend.CapTTS)
	if err != nil {
		t.Fatalf("expected PASS, got error: %v", err)
	}
}

func TestGate_UnsupportedCapability_ReturnsErrCapabilityUnsupported(t *testing.T) {
	be := befake.NewWithID("test-backend")
	ctx := context.Background()
	be.Start(ctx, backend.StartConfig{})

	reg := backend.NewRegistry()
	reg.MustRegister("test-backend", func() backend.Backend { return be })
	mgr := backend.NewManager(reg)
	mgr.Select("test-backend")

	gate := NewDefaultGate(mgr)
	err := gate.Check(ctx, backend.CapVoiceClone)
	if err == nil {
		t.Fatal("expected error for unsupported capability")
	}
	if err != ErrCapabilityUnsupported {
		t.Errorf("expected ErrCapabilityUnsupported, got %v", err)
	}
}

func TestGate_NoActiveBackend_ReturnsErrNoActiveBackend(t *testing.T) {
	reg := backend.NewRegistry()
	mgr := backend.NewManager(reg)

	gate := NewDefaultGate(mgr)
	err := gate.Check(context.Background(), backend.CapTTS)
	if err == nil {
		t.Fatal("expected error for no active backend")
	}
	if err != ErrNoActiveBackend {
		t.Errorf("expected ErrNoActiveBackend, got %v", err)
	}
}

func TestGate_BackendNotReady_ReturnsErrBackendNotReady(t *testing.T) {
	be := befake.NewWithID("test-backend")
	// Don't start the backend, so it stays in StateStopped

	reg := backend.NewRegistry()
	reg.MustRegister("test-backend", func() backend.Backend { return be })
	mgr := backend.NewManager(reg)
	mgr.Select("test-backend")

	gate := NewDefaultGate(mgr)
	err := gate.Check(context.Background(), backend.CapTTS)
	if err == nil {
		t.Fatal("expected error for backend not ready")
	}
	if err != ErrBackendNotReady {
		t.Errorf("expected ErrBackendNotReady, got %v", err)
	}
}
