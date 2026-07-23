package audiocpp

import (
	"context"
	"testing"
	"time"

	core "github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
)

// ---------------------------------------------------------------------------
// mapProcessState 單元測試
// ---------------------------------------------------------------------------

func TestMapProcessState(t *testing.T) {
	tests := []struct {
		name  string
		input core.ProcessState
		want  backend.State
	}{
		{"Stopped", core.StateStopped, backend.StateStopped},
		{"Starting", core.StateStarting, backend.StateStarting},
		{"Running", core.StateRunning, backend.StateRunning},
		{"Stopping", core.StateStopping, backend.StateStopping},
		{"Crashed", core.StateCrashed, backend.StateCrashed},
		{"Unknown (default)", core.ProcessState(99), backend.StateStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapProcessState(tt.input)
			if got != tt.want {
				t.Errorf("mapProcessState(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Adapter 初始狀態測試
// ---------------------------------------------------------------------------

func TestAdapter_InitialState(t *testing.T) {
	// 建立一個未啟動的 Adapter
	proc := core.NewProcess("", "", "", "127.0.0.1", 19999, 0, 1, "", false, 0)
	client := core.NewClient("127.0.0.1", 19999, 2*time.Second)
	adapter := New(proc, client)

	if s := adapter.State(); s != backend.StateStopped {
		t.Errorf("initial State() = %v, want %v", s, backend.StateStopped)
	}
}

// ---------------------------------------------------------------------------
// WaitForReady — Context 取消測試
// ---------------------------------------------------------------------------

func TestAudioCppAdapter_WaitForReady_ContextCanceled(t *testing.T) {
	proc := core.NewProcess("", "", "", "127.0.0.1", 19998, 0, 1, "", false, 0)
	client := core.NewClient("127.0.0.1", 19998, 1*time.Second)
	adapter := New(proc, client)

	// 用一個已取消的 context → proc.WaitForReady 立即返回 ctx.Err()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err := adapter.WaitForReady(ctx, 5)
	if err == nil {
		t.Fatal("WaitForReady should return error when context is canceled")
	}

	t.Logf("Got expected error: %v", err)

	// 狀態不應是 Running
	if s := adapter.State(); s == backend.StateRunning {
		t.Error("State should not be Running after failed WaitForReady")
	}
}

// ---------------------------------------------------------------------------
// WaitForReady — Context 逾時測試
// ---------------------------------------------------------------------------

func TestAudioCppAdapter_WaitForReady_HTTPHealthTimeout(t *testing.T) {
	proc := core.NewProcess("", "", "", "127.0.0.1", 19997, 0, 1, "", false, 0)
	client := core.NewClient("127.0.0.1", 19997, 1*time.Second)
	adapter := New(proc, client)

	// 極短的 timeout → proc.WaitForReady 會因為 TCP 連不上而逾時
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := adapter.WaitForReady(ctx, 1)
	if err == nil {
		t.Fatal("WaitForReady should return error when context times out")
	}

	t.Logf("Got expected error: %v", err)

	if s := adapter.State(); s == backend.StateRunning {
		t.Error("State should not be Running after failed WaitForReady")
	}
}

// ---------------------------------------------------------------------------
// WaitForReady — 失敗後狀態不為 Running
// ---------------------------------------------------------------------------

func TestAudioCppAdapter_WaitForReady_DoesNotSetRunningOnFailure(t *testing.T) {
	proc := core.NewProcess("", "", "", "127.0.0.1", 19996, 0, 1, "", false, 0)
	client := core.NewClient("127.0.0.1", 19996, 1*time.Second)
	adapter := New(proc, client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_ = adapter.WaitForReady(ctx, 1)

	// 不論何種原因失敗，State 都不應為 Running
	if s := adapter.State(); s == backend.StateRunning {
		t.Error("State must not be Running when WaitForReady fails")
	}
}

// ---------------------------------------------------------------------------
// WaitForReady — 失敗後 cleanup child（透過 State 變為 Stopped 驗證）
// ---------------------------------------------------------------------------

func TestAudioCppAdapter_WaitForReady_CleansChildOnFailure(t *testing.T) {
	proc := core.NewProcess("", "", "", "127.0.0.1", 19995, 0, 1, "", false, 0)
	client := core.NewClient("127.0.0.1", 19995, 1*time.Second)
	adapter := New(proc, client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	// 第一次 WaitForReady 會觸發 proc.WaitForReady → TCP 連不上 → proc.Stop()
	// 由於從未 Start，proc.WaitForReady 中 WaitForServer 超時後會呼叫 p.Stop()
	err := adapter.WaitForReady(ctx, 1)
	if err == nil {
		t.Fatal("WaitForReady should fail when no server is running")
	}

	// 如果 proc.Stop() 被呼叫，state 應該是 Stopped
	s := adapter.State()
	if s == backend.StateRunning {
		t.Error("State must not be Running; proc.Stop() should have been called")
	}

	// 若 state 為 Stopped 則表示 cleanup 執行完畢
	t.Logf("State after failed WaitForReady: %v (expected Stopped or Crashed)", s)
}

// ---------------------------------------------------------------------------
// Adapter — 未啟動時各方法行為
// ---------------------------------------------------------------------------

func TestAdapter_Capabilities(t *testing.T) {
	proc := core.NewProcess("", "", "", "127.0.0.1", 19994, 0, 1, "", false, 0)
	client := core.NewClient("127.0.0.1", 19994, 1*time.Second)
	adapter := New(proc, client, backend.CapASR, backend.CapTTS)

	t.Run("Capabilities returns configured capabilities", func(t *testing.T) {
		caps := adapter.Capabilities()
		if len(caps) != 2 {
			t.Fatalf("expected 2 capabilities, got %d", len(caps))
		}
		if caps[0] != backend.CapASR {
			t.Errorf("expected CapASR first, got %s", caps[0])
		}
		if caps[1] != backend.CapTTS {
			t.Errorf("expected CapTTS second, got %s", caps[1])
		}
	})

	t.Run("Capabilities returns copy", func(t *testing.T) {
		caps := adapter.Capabilities()
		caps[0] = backend.CapVAD
		// original should be unchanged
		if adapter.Capabilities()[0] != backend.CapASR {
			t.Error("Capabilities should return a copy")
		}
	})
}

func TestAdapter_ID(t *testing.T) {
	proc := core.NewProcess("", "", "", "127.0.0.1", 19993, 0, 1, "", false, 0)
	client := core.NewClient("127.0.0.1", 19993, 1*time.Second)
	adapter := New(proc, client)

	if id := adapter.ID(); id != "audiocpp" {
		t.Errorf("ID() = %q, want %q", id, "audiocpp")
	}
}

func TestAdapter_PID(t *testing.T) {
	proc := core.NewProcess("", "", "", "127.0.0.1", 19992, 0, 1, "", false, 0)
	client := core.NewClient("127.0.0.1", 19992, 1*time.Second)
	adapter := New(proc, client)

	if pid := adapter.PID(); pid != -1 {
		t.Errorf("PID() before start should be -1, got %d", pid)
	}
}

func TestAdapter_Diagnostics(t *testing.T) {
	proc := core.NewProcess("", "", "", "127.0.0.1", 19991, 0, 1, "", false, 0)
	client := core.NewClient("127.0.0.1", 19991, 1*time.Second)
	adapter := New(proc, client)

	d := adapter.Diagnostics()
	if d.BackendID != "audiocpp" {
		t.Errorf("Diagnostics.BackendID = %q, want %q", d.BackendID, "audiocpp")
	}
	if d.PID != -1 {
		t.Errorf("Diagnostics.PID = %d, want -1", d.PID)
	}
	if d.State != backend.StateStopped {
		t.Errorf("Diagnostics.State = %v, want Stopped", d.State)
	}
}
