package audiocpp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/platform"
)

type ServerModelConfig struct {
	ID                 string                 `json:"id"`
	Path               string                 `json:"path"`
	Family             string                 `json:"family"`
	Task               string                 `json:"task,omitempty"`
	Mode               string                 `json:"mode,omitempty"`
	Lazy               bool                   `json:"lazy,omitempty"`
	VoicePresets       map[string]VoicePreset `json:"voice_presets,omitempty"`
	DefaultVoicePreset interface{}            `json:"default_voice_preset,omitempty"`
	LoadOptions        map[string]string      `json:"load_options,omitempty"`
	SessionOptions     map[string]string      `json:"session_options,omitempty"`
}

type VoicePreset struct {
	VoiceID       string `json:"voice_id,omitempty"`
	VoiceRef      string `json:"voice_ref,omitempty"`
	ReferenceText string `json:"reference_text,omitempty"`
}

type ServerConfigJSON struct {
	Host    string              `json:"host"`
	Port    int                 `json:"port"`
	Backend string              `json:"backend"`
	Device  int                 `json:"device"`
	Threads int                 `json:"threads"`
	Models  []ServerModelConfig `json:"models"`
}

type ProcessState int32

const (
	StateStopped  ProcessState = 0
	StateStarting ProcessState = 1
	StateRunning  ProcessState = 2
	StateStopping ProcessState = 3
	StateCrashed  ProcessState = 4
)

type Process struct {
	serverPath string
	workingDir string
	host       string
	port       int
	backend    string
	device     int
	threads    int

	config       *ServerConfigJSON
	configPath   string
	restartCount int
	maxRestarts  int
	autoRestart  bool

	mu     sync.Mutex
	cmd    *exec.Cmd
	state  atomic.Int32
	stopCh chan struct{}
	doneCh chan struct{}
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func NewProcess(cfgDir, serverPath, workingDir, host string, port, device, threads int, backend string, autoRestart bool, maxRestarts int) *Process {
	configPath := filepath.Join(os.TempDir(), fmt.Sprintf("audiocpp_server_%d.json", port))
	if cfgDir != "" {
		configPath = filepath.Join(cfgDir, "audiocpp_server.json")
	}

	p := &Process{
		serverPath:  serverPath,
		workingDir:  workingDir,
		host:        host,
		port:        port,
		backend:     backend,
		device:      device,
		threads:     threads,
		configPath:  configPath,
		maxRestarts: maxRestarts,
		autoRestart: autoRestart,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
	p.state.Store(int32(StateStopped))
	return p
}

func (p *Process) SetModelConfig(models []ServerModelConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config = &ServerConfigJSON{
		Host:    p.host,
		Port:    p.port,
		Backend: p.backend,
		Device:  p.device,
		Threads: p.threads,
		Models:  models,
	}
}

func (p *Process) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state.Load() != int32(StateStopped) && p.state.Load() != int32(StateCrashed) {
		return fmt.Errorf("process already running (state=%d)", p.state.Load())
	}

	configData, err := json.MarshalIndent(p.config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(p.configPath), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	if err := os.WriteFile(p.configPath, configData, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	log.Printf("[audiocpp] starting server: %s --config %s --host %s --port %d --backend %s --device %d --threads %d",
		p.serverPath, p.configPath, p.host, p.port, p.backend, p.device, p.threads)

	args := []string{
		"--config", p.configPath,
		"--host", p.host,
		"--port", strconv.Itoa(p.port),
		"--backend", p.backend,
		"--device", strconv.Itoa(p.device),
		"--threads", strconv.Itoa(p.threads),
	}

	cmd := exec.CommandContext(ctx, p.serverPath, args...)
	if p.workingDir != "" {
		cmd.Dir = p.workingDir
	}

	platform.SetProcessGroup(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	p.stdout = stdout

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}
	p.stderr = stderr

	if err := cmd.Start(); err != nil {
		p.state.Store(int32(StateCrashed))
		return fmt.Errorf("start process: %w", err)
	}

	p.cmd = cmd
	p.state.Store(int32(StateStarting))

	go p.monitorOutput(stdout, stderr)
	go p.monitorProcess(ctx)

	return nil
}

func (p *Process) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		state := ProcessState(p.state.Load())
		if state == StateRunning {
			return nil
		}
		if state == StateCrashed || state == StateStopped {
			return fmt.Errorf("process stopped unexpectedly during startup")
		}

		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for server to become ready")
}

func (p *Process) MarkReady() {
	p.state.Store(int32(StateRunning))
}

func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	state := ProcessState(p.state.Load())
	if state == StateStopped {
		return nil
	}

	p.state.Store(int32(StateStopping))

	if p.cmd != nil && p.cmd.Process != nil {
		log.Printf("[audiocpp] stopping server (pid=%d)", p.cmd.Process.Pid)
		if err := platform.KillProcessTree(p.cmd.Process.Pid); err != nil {
			log.Printf("[audiocpp] kill process tree error: %v", err)
		}
	}

	close(p.stopCh)
	<-p.doneCh

	p.state.Store(int32(StateStopped))
	return nil
}

func (p *Process) Restart(ctx context.Context) error {
	log.Printf("[audiocpp] restarting server")
	if err := p.Stop(); err != nil {
		log.Printf("[audiocpp] stop error during restart: %v", err)
	}

	restartCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return p.Start(restartCtx)
}

func (p *Process) State() ProcessState {
	return ProcessState(p.state.Load())
}

func (p *Process) IsRunning() bool {
	return ProcessState(p.state.Load()) == StateRunning
}

func (p *Process) ConfigPath() string {
	return p.configPath
}

func (p *Process) monitorOutput(stdout, stderr io.ReadCloser) {
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				log.Printf("[audiocpp:stdout] %s", string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				log.Printf("[audiocpp:stderr] %s", string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()
}

func (p *Process) monitorProcess(ctx context.Context) {
	defer func() {
		p.doneCh <- struct{}{}
	}()

	err := p.cmd.Wait()
	if err != nil {
		if ctx.Err() != nil {
			log.Printf("[audiocpp] server process terminated (context cancelled)")
			return
		}
		log.Printf("[audiocpp] server process exited: %v", err)
	}

	state := ProcessState(p.state.Load())
	if state == StateStopping || state == StateStopped {
		return
	}

	p.state.Store(int32(StateCrashed))
	log.Printf("[audiocpp] server process crashed")

	if p.autoRestart && p.restartCount < p.maxRestarts {
		p.restartCount++
		log.Printf("[audiocpp] auto-restart attempt %d/%d", p.restartCount, p.maxRestarts)
		if err := p.Restart(ctx); err != nil {
			log.Printf("[audiocpp] auto-restart failed: %v", err)
		}
	}
}

func (p *Process) Pid() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}
