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
	Host              string              `json:"host"`
	Port              int                 `json:"port"`
	Backend           string              `json:"backend"`
	Device            int                 `json:"device"`
	Threads           int                 `json:"threads"`
	LazyLoad          bool                `json:"lazy_load"`
	ModelSpecOverride string              `json:"model_spec_override,omitempty"`
	Models            []ServerModelConfig `json:"models"`
}

type ProcessState int32

const (
	StateStopped  ProcessState = 0
	StateStarting ProcessState = 1
	StateRunning  ProcessState = 2
	StateStopping ProcessState = 3
	StateCrashed  ProcessState = 4
)

type generation struct {
	cmd    *exec.Cmd
	stopCh chan struct{}
	doneCh chan struct{}
	cancel context.CancelFunc
	stdout io.ReadCloser
	stderr io.ReadCloser
	pid    int
	exited atomic.Bool
	state  atomic.Int32
}

type Process struct {
	cfgDir            string
	serverPath        string
	workingDir        string
	host              string
	port              int
	backend           string
	device            int
	threads           int
	maxRestarts       int
	autoRestart       bool
	configPath        string
	modelSpecOverride string
	lazyLoad          bool

	config     *ServerConfigJSON
	configLock sync.Mutex

	genMu        sync.Mutex
	current      *generation
	restartCount int
	stopping     atomic.Bool
	state        atomic.Int32

	ExtraEnv []string
}

func NewProcess(cfgDir, serverPath, workingDir, host string, port, device, threads int, backend string, autoRestart bool, maxRestarts int) *Process {
	configPath := filepath.Join(os.TempDir(), fmt.Sprintf("audiocpp_server_%d.json", port))
	if cfgDir != "" {
		configPath = filepath.Join(cfgDir, "audiocpp_server.json")
	}

	p := &Process{
		cfgDir:      cfgDir,
		serverPath:  serverPath,
		workingDir:  workingDir,
		host:        host,
		port:        port,
		backend:     backend,
		device:      device,
		threads:     threads,
		maxRestarts: maxRestarts,
		autoRestart: autoRestart,
		configPath:  configPath,
	}
	p.state.Store(int32(StateStopped))
	return p
}

func (p *Process) SetModelSpecOverride(path string) {
	p.modelSpecOverride = path
}

func (p *Process) SetLazyLoad(v bool) {
	p.lazyLoad = v
}

func (p *Process) SetModelConfig(models []ServerModelConfig) {
	p.configLock.Lock()
	defer p.configLock.Unlock()
	p.config = &ServerConfigJSON{
		Host:              p.host,
		Port:              p.port,
		Backend:           p.backend,
		Device:            p.device,
		Threads:           p.threads,
		LazyLoad:          p.lazyLoad,
		ModelSpecOverride: p.modelSpecOverride,
		Models:            models,
	}
}

func (p *Process) Start(lifetimeCtx context.Context) error {
	if lifetimeCtx == nil {
		return fmt.Errorf("lifetime context must not be nil")
	}

	p.genMu.Lock()
	defer p.genMu.Unlock()

	if p.current != nil && !p.current.exited.Load() {
		return fmt.Errorf("already running")
	}

	p.restartCount = 0
	p.stopping.Store(false)

	gen, err := p.newGeneration(lifetimeCtx)
	if err != nil {
		return fmt.Errorf("create generation: %w", err)
	}

	if err := gen.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}
	gen.pid = gen.cmd.Process.Pid
	gen.state.Store(int32(StateStarting))
	p.state.Store(int32(StateStarting))

	log.Printf("[audiocpp] started server (pid=%d)", gen.pid)
	p.current = gen

	go p.monitorOutput(gen.stdout, gen.stderr)
	go p.supervise(lifetimeCtx, gen)

	return nil
}

func (p *Process) WaitForReady(ctx context.Context, timeout time.Duration) error {
	err := WaitForServer(ctx, p.host, p.port, timeout)
	if err != nil {
		p.Stop()
		return fmt.Errorf("server not ready: %w", err)
	}

	gen := p.getCurrentGen()
	if gen != nil {
		gen.state.Store(int32(StateRunning))
	}
	p.state.Store(int32(StateRunning))
	return nil
}

func (p *Process) Stop() error {
	p.stopping.Store(true)

	p.genMu.Lock()
	gen := p.current
	p.current = nil
	p.genMu.Unlock()

	if gen == nil {
		return nil
	}

	if gen.cmd != nil && gen.cmd.Process != nil {
		log.Printf("[audiocpp] stopping server (pid=%d)", gen.pid)
		if err := platform.KillProcessTree(gen.pid); err != nil {
			log.Printf("[audiocpp] kill process tree error: %v", err)
		}
	}

	select {
	case <-gen.stopCh:
	default:
		close(gen.stopCh)
	}

	tctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	select {
	case <-gen.doneCh:
	case <-tctx.Done():
		log.Printf("[audiocpp] stop timeout waiting for supervisor")
	}

	p.state.Store(int32(StateStopped))
	return nil
}

// StopGraceful sends a graceful stop signal to the child process and waits up to
// 5 seconds for it to exit on its own. It returns true if the process exited
// cleanly within the deadline, or false if force termination is required.
func (p *Process) StopGraceful() bool {
	p.stopping.Store(true)

	p.genMu.Lock()
	gen := p.current
	// Keep current reference for now so supervise doesn't restart
	p.genMu.Unlock()

	if gen == nil {
		return true
	}

	if gen.cmd != nil && gen.cmd.Process != nil {
		log.Printf("[audiocpp] sending graceful stop to (pid=%d)", gen.pid)
		if err := platform.StopGraceful(gen.pid); err != nil {
			log.Printf("[audiocpp] graceful stop error: %v", err)
		}
	}

	// Wait up to 5 seconds for the process to exit on its own
	gracefulDeadline := 5 * time.Second
	exited := platform.WaitProcessExit(gen.pid, gracefulDeadline)
	if exited {
		log.Printf("[audiocpp] process (pid=%d) exited gracefully", gen.pid)

		p.genMu.Lock()
		if p.current == gen {
			p.current = nil
		}
		p.genMu.Unlock()

		select {
		case <-gen.stopCh:
		default:
			close(gen.stopCh)
		}

		p.state.Store(int32(StateStopped))
		return true
	}

	log.Printf("[audiocpp] process (pid=%d) did not exit gracefully within deadline", gen.pid)
	return false
}

// ForceStop immediately kills the child process tree and sets state to Stopped.
// Unlike Stop() which also uses KillProcessTree, ForceStop uses a shorter
// supervisor deadline and is suitable for tests that need aggressive cleanup.
func (p *Process) ForceStop() error {
	p.stopping.Store(true)

	p.genMu.Lock()
	gen := p.current
	p.current = nil
	p.genMu.Unlock()

	if gen == nil {
		p.state.Store(int32(StateStopped))
		return nil
	}

	if gen.cmd != nil && gen.cmd.Process != nil {
		log.Printf("[audiocpp] force stopping server (pid=%d)", gen.pid)
		if err := platform.KillProcessTree(gen.pid); err != nil {
			log.Printf("[audiocpp] force kill error: %v", err)
		}
	}

	gen.cancel()

	select {
	case <-gen.stopCh:
	default:
		close(gen.stopCh)
	}

	tctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	select {
	case <-gen.doneCh:
	case <-tctx.Done():
		log.Printf("[audiocpp] force stop timeout waiting for supervisor")
	}

	p.state.Store(int32(StateStopped))
	return nil
}

func (p *Process) Restart(lifetimeCtx context.Context) error {
	log.Printf("[audiocpp] restarting server")
	if err := p.Stop(); err != nil {
		log.Printf("[audiocpp] stop error during restart: %v", err)
	}

	if err := p.Start(lifetimeCtx); err != nil {
		return fmt.Errorf("restart start failed: %w", err)
	}

	startupCtx, cancel := context.WithTimeout(lifetimeCtx, 30*time.Second)
	defer cancel()
	return p.WaitForReady(startupCtx, 30*time.Second)
}

func (p *Process) MarkReady() {
	gen := p.getCurrentGen()
	if gen != nil {
		gen.state.Store(int32(StateRunning))
	}
	p.state.Store(int32(StateRunning))
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

func (p *Process) Pid() int {
	gen := p.getCurrentGen()
	if gen != nil {
		return gen.pid
	}
	return 0
}

func (p *Process) getCurrentGen() *generation {
	p.genMu.Lock()
	defer p.genMu.Unlock()
	return p.current
}

func (p *Process) newGeneration(lifetimeCtx context.Context) (*generation, error) {
	p.configLock.Lock()
	config := p.config
	p.configLock.Unlock()

	if config == nil {
		return nil, fmt.Errorf("model config not set, call SetModelConfig first")
	}

	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(p.configPath), 0755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	if err := os.WriteFile(p.configPath, configData, 0644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}

	args := []string{
		"--config", p.configPath,
		"--host", p.host,
		"--port", strconv.Itoa(p.port),
		"--backend", p.backend,
		"--device", strconv.Itoa(p.device),
		"--threads", strconv.Itoa(p.threads),
	}

	genCtx, genCancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(genCtx, p.serverPath, args...)
	if p.workingDir != "" {
		cmd.Dir = p.workingDir
	}

	platform.SetProcessGroup(cmd)

	if len(p.ExtraEnv) > 0 {
		cmd.Env = append(os.Environ(), p.ExtraEnv...)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		genCancel()
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		genCancel()
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	return &generation{
		cmd:    cmd,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
		cancel: genCancel,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

func (p *Process) supervise(ctx context.Context, gen *generation) {
	defer close(gen.doneCh)

	procDone := make(chan error, 1)
	go func() {
		procDone <- gen.cmd.Wait()
	}()

	select {
	case <-gen.stopCh:
		<-procDone
		return

	case <-procDone:
		gen.exited.Store(true)

		select {
		case <-gen.stopCh:
			return
		default:
		}

		if ctx.Err() != nil {
			p.state.Store(int32(StateStopped))
			return
		}

		p.genMu.Lock()
		isCurrent := p.current == gen
		p.genMu.Unlock()

		if !isCurrent || p.stopping.Load() {
			return
		}

		if p.autoRestart && p.restartCount < p.maxRestarts {
			log.Printf("[audiocpp] server exited, auto-restart (%d/%d)",
				p.restartCount+1, p.maxRestarts)

			newGen, err := p.newGeneration(ctx)
			if err != nil {
				log.Printf("[audiocpp] auto-restart create generation failed: %v", err)
				p.state.Store(int32(StateCrashed))
				return
			}

			if err := newGen.cmd.Start(); err != nil {
				log.Printf("[audiocpp] auto-restart start failed: %v", err)
				close(newGen.doneCh)
				p.state.Store(int32(StateCrashed))
				return
			}
			newGen.pid = newGen.cmd.Process.Pid
			newGen.state.Store(int32(StateStarting))

			go p.monitorOutput(newGen.stdout, newGen.stderr)

			p.genMu.Lock()
			if p.stopping.Load() || p.current != gen {
				log.Printf("[audiocpp] auto-restart aborted by Stop")
				p.genMu.Unlock()
				platform.KillProcessTree(newGen.pid)
				close(newGen.doneCh)
				if !p.stopping.Load() {
					p.state.Store(int32(StateStopped))
				}
				return
			}
			p.restartCount++
			p.current = newGen
			p.genMu.Unlock()

			log.Printf("[audiocpp] auto-restart succeeded (pid=%d)", newGen.pid)
			p.supervise(ctx, newGen)
			return
		}

		p.state.Store(int32(StateCrashed))
		log.Printf("[audiocpp] server exited, no more restarts")
	}
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
