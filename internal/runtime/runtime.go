package runtime

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/backend"
	audiocppadapter "github.com/liuxb99/audiocpp-runtime-go/internal/backend/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/config"
	"github.com/liuxb99/audiocpp-runtime-go/internal/execution"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/models"
	"github.com/liuxb99/audiocpp-runtime-go/internal/outputs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/platform"
	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

type Runtime struct {
	mu      sync.RWMutex
	stateMu sync.Mutex
	state   RuntimeState
	cfg     *config.Config
	proc    *audiocpp.Process
	cli     *audiocpp.CLIExecutor
	client  *audiocpp.Client

	backendMgr *backend.Manager

	db         *storage.DB
	jobRepo    *storage.JobsRepository
	outputRepo *storage.OutputsRepository

	modelReg   *models.Registry
	outputMgr  *outputs.Manager
	jobMgr     *jobs.Manager
	workerPool *jobs.WorkerPool

	lifetimeCtx context.Context
	lifetimeCnl context.CancelFunc
	startTime   time.Time

	readyTime      time.Time
	childStartTime time.Time
	shutdownTime   time.Time

	lastSchedule *ShutdownSchedule

	httpShutdownFn func(timeout time.Duration) error
}

// ShutdownResult captures the outcome of a Runtime.Shutdown call.
type ShutdownResult struct {
	RequestAccepted bool `json:"request_accepted"`
	GracefulExited  bool `json:"graceful_exited"`
	ForceKillUsed   bool `json:"force_kill_used"`
	RuntimeExited   bool `json:"runtime_exited"`
	ChildExited     bool `json:"child_exited"`
}

func New(cfg *config.Config) *Runtime {
	r := &Runtime{
		cfg:       cfg,
		startTime: time.Now(),
		state:     StateCreated,
	}
	return r
}

func (r *Runtime) Init(ctx context.Context) error {
	log.Printf("[runtime] initializing")

	if err := r.transition(StateCreated, StateInitializing); err != nil {
		return fmt.Errorf("state transition: %w", err)
	}

	db, err := storage.NewDB(r.cfg.Storage.SqlitePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	r.db = db

	if err := db.RunMigrations(); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	r.jobRepo = storage.NewJobsRepository(db)
	r.outputRepo = storage.NewOutputsRepository(db)

	r.client = audiocpp.NewClient(r.cfg.AudioCpp.Host, r.cfg.AudioCpp.Port,
		time.Duration(r.cfg.AudioCpp.RequestTimeoutSec)*time.Second)

	r.cli = audiocpp.NewCLIExecutor(r.cfg.AudioCpp.CliPath, r.cfg.AudioCpp.WorkingDir,
		time.Duration(r.cfg.AudioCpp.RequestTimeoutSec)*time.Second)

	r.modelReg = models.NewRegistry(r.cfg.Models.RegistryPath)
	if err := r.modelReg.Load(); err != nil {
		log.Printf("[runtime] warning: could not load model registry: %v", err)
	}

	r.outputMgr = outputs.NewManager(r.cfg.Outputs.RootDir, r.cfg.Outputs.RetainDays, r.outputRepo)
	r.jobMgr = jobs.NewManager(r.jobRepo, r.cfg.Jobs.QueueCapacity)

	r.proc = audiocpp.NewProcess(
		"",
		r.cfg.AudioCpp.ServerPath,
		r.cfg.AudioCpp.WorkingDir,
		r.cfg.AudioCpp.Host,
		r.cfg.AudioCpp.Port,
		r.cfg.AudioCpp.Device,
		r.cfg.AudioCpp.Threads,
		r.cfg.AudioCpp.Backend,
		r.cfg.AudioCpp.AutoRestart,
		r.cfg.AudioCpp.MaxRestartAttempts,
	)

	if r.cfg.AudioCpp.ModelSpecOverride != "" {
		r.proc.SetModelSpecOverride(r.cfg.AudioCpp.ModelSpecOverride)
	}
	r.proc.SetLazyLoad(r.cfg.AudioCpp.LazyLoad)

	if len(r.cfg.AudioCpp.Models) > 0 {
		models := make([]audiocpp.ServerModelConfig, len(r.cfg.AudioCpp.Models))
		for i, m := range r.cfg.AudioCpp.Models {
			presets := make(map[string]audiocpp.VoicePreset)
			for k, v := range m.VoicePresets {
				presets[k] = audiocpp.VoicePreset{
					VoiceID:       v.VoiceID,
					VoiceRef:      v.VoiceRef,
					ReferenceText: v.ReferenceText,
				}
			}
			models[i] = audiocpp.ServerModelConfig{
				ID:                 m.ID,
				Path:               m.Path,
				Family:             m.Family,
				Task:               m.Task,
				Mode:               m.Mode,
				Lazy:               m.Lazy,
				VoicePresets:       presets,
				DefaultVoicePreset: m.DefaultVoicePreset,
				LoadOptions:        m.LoadOptions,
				SessionOptions:     m.SessionOptions,
			}
		}
		r.proc.SetModelConfig(models)
	}

	r.lifetimeCtx, r.lifetimeCnl = context.WithCancel(ctx)

	// 初始化 Backend Manager（使用專屬 Registry，避免跨測試共享 builder）
	reg := backend.NewRegistry()
	r.backendMgr = backend.NewManager(reg)
	builder := audiocppadapter.NewBuilder(r.proc, r.client, backend.CapASR, backend.CapTTS, backend.CapVoiceClone, backend.CapVAD, backend.CapDiarization, backend.CapAlignment)
	reg.MustRegister("audiocpp", builder)
	if err := r.backendMgr.Select("audiocpp"); err != nil {
		return fmt.Errorf("select backend: %w", err)
	}

	log.Printf("[runtime] initialization complete")
	return nil
}

func (r *Runtime) StartAudioCpp(ctx context.Context) error {
	if r.backendMgr == nil {
		return fmt.Errorf("backend manager not initialized")
	}

	if err := r.transition(StateInitializing, StateStarting); err != nil {
		return fmt.Errorf("state transition: %w", err)
	}

	log.Printf("[runtime] starting audiocpp server")

	cfg := backend.StartConfig{
		Device:   r.cfg.AudioCpp.Device,
		Threads:  r.cfg.AudioCpp.Threads,
		LazyLoad: r.cfg.AudioCpp.LazyLoad,
	}

	readyCtx, cancel := context.WithTimeout(r.lifetimeCtx,
		time.Duration(r.cfg.AudioCpp.StartupTimeoutSec)*time.Second)
	defer cancel()

	if err := r.backendMgr.StartAndWait(readyCtx, cfg, r.cfg.AudioCpp.StartupTimeoutSec); err != nil {
		return fmt.Errorf("start backend: %w", err)
	}

	r.childStartTime = time.Now()
	r.readyTime = time.Now()
	log.Printf("[runtime] audiocpp server is ready")

	if err := r.transition(StateStarting, StateReady); err != nil {
		return fmt.Errorf("state transition: %w", err)
	}

	if err := r.modelReg.Refresh(ctx, r.client); err != nil {
		log.Printf("[runtime] warning: model registry refresh failed: %v", err)
	}

	return nil
}

func (r *Runtime) StartWorkers(count int) {
	// 建立 DefaultExecutor，透過 BackendManager 執行 Job
	mapper := execution.NewDefaultMapper()
	gate := execution.NewDefaultGate(r.backendMgr)
	executor := execution.NewDefaultExecutor(r.backendMgr, mapper, gate)
	jobExecutor := execution.NewJobExecutorAdapter(executor)

	r.workerPool = jobs.NewWorkerPool(r.jobMgr, jobExecutor, count)
	// Apply configuration from config
	timeout := time.Duration(r.cfg.Jobs.DefaultTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	r.workerPool.WithConfig(
		timeout,
		r.cfg.Jobs.MaxAttempts,
		time.Duration(r.cfg.Jobs.RetryInitialDelayMs)*time.Millisecond,
		time.Duration(r.cfg.Jobs.RetryMaxDelayMs)*time.Millisecond,
	)
	r.workerPool.Start()

	if err := r.transition(StateReady, StateRunning); err != nil {
		log.Printf("[runtime] warning: state transition to running: %v", err)
	}
}

func (r *Runtime) StopWorkers() {
	if r.workerPool != nil {
		r.workerPool.Stop()
	}
}

func (r *Runtime) Shutdown(ctx context.Context) ShutdownResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.shutdownTime = time.Now()
	schedule := NewShutdownSchedule()

	result := ShutdownResult{
		RequestAccepted: true,
	}

	// Step 0: check current state — if already stopped, return immediately
	r.stateMu.Lock()
	currentState := r.state
	r.stateMu.Unlock()

	if currentState == StateStopped {
		log.Printf("[runtime] already stopped, shutdown skipped")
		result.RuntimeExited = true
		result.ChildExited = true
		r.lastSchedule = schedule
		return result
	}

	// Transition to Stopping
	if err := r.transitionLocked(StateCreated, StateStopping); err != nil {
		// try other valid from-states
		_ = r.transitionFromAnyToStoppingLocked()
	}

	log.Printf("[runtime] shutting down")

	// Step 1: RequestAccepted
	schedule.ExecuteStep(StepRequestAccepted, func() error {
		return nil
	}, 0)

	// Step 2: StopWorkers
	schedule.ExecuteStep(StepStopWorkers, func() error {
		r.StopWorkers()
		return nil
	}, 5*time.Second)

	// Step 3: FlushQueue — wait for job queue to drain
	schedule.ExecuteStep(StepFlushQueue, func() error {
		if r.jobMgr != nil {
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if r.jobMgr.QueueLen() == 0 {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		return nil
	}, 5*time.Second)

	// Step 4: StopChild
	childPID := 0
	schedule.ExecuteStep(StepStopChild, func() error {
		if r.backendMgr == nil {
			return nil
		}
		childPID = r.backendMgr.PID()
		return r.backendMgr.Stop()
	}, 10*time.Second)

	if childPID > 0 {
		result.GracefulExited = !platform.ProcessExists(childPID)
		result.ForceKillUsed = !result.GracefulExited
		result.ChildExited = !platform.ProcessExists(childPID)
	} else {
		result.ChildExited = true
	}

	// Cancel lifetime context
	if r.lifetimeCnl != nil {
		r.lifetimeCnl()
	}

	// Step 5: SaveState
	schedule.ExecuteStep(StepSaveState, func() error {
		if r.modelReg != nil {
			return r.modelReg.Save()
		}
		return nil
	}, 5*time.Second)

	// Step 6: CloseDB
	schedule.ExecuteStep(StepCloseDB, func() error {
		if r.db != nil {
			return r.db.Close()
		}
		return nil
	}, 5*time.Second)

	// Step 7: StopHTTP
	schedule.ExecuteStep(StepStopHTTP, func() error {
		if r.httpShutdownFn != nil {
			return r.httpShutdownFn(10 * time.Second)
		}
		return nil
	}, 10*time.Second)

	// Step 8: ExitMain
	schedule.ExecuteStep(StepExitMain, func() error {
		return nil
	}, 0)

	result.RuntimeExited = true
	r.lastSchedule = schedule

	// Transition to Stopped
	r.stateMu.Lock()
	r.state = StateStopped
	r.stateMu.Unlock()

	log.Printf("[runtime] shutdown complete")
	return result
}

func (r *Runtime) transitionFromAnyToStoppingLocked() error {
	allowedSources := []RuntimeState{
		StateCreated, StateInitializing, StateStarting, StateReady, StateRunning,
	}
	for _, src := range allowedSources {
		if r.state == src {
			r.state = StateStopping
			return nil
		}
	}
	return fmt.Errorf("cannot transition from %s to stopping", StateString(r.state))
}

func (r *Runtime) SetHTTPServerShutdownFn(fn func(timeout time.Duration) error) {
	r.httpShutdownFn = fn
}

func (r *Runtime) LastShutdownSchedule() *ShutdownSchedule {
	return r.lastSchedule
}

func (r *Runtime) Client() *audiocpp.Client {
	return r.client
}

func (r *Runtime) CLI() *audiocpp.CLIExecutor {
	return r.cli
}

func (r *Runtime) Process() *audiocpp.Process {
	return r.proc
}

func (r *Runtime) ModelRegistry() *models.Registry {
	return r.modelReg
}

func (r *Runtime) OutputManager() *outputs.Manager {
	return r.outputMgr
}

func (r *Runtime) JobManager() *jobs.Manager {
	return r.jobMgr
}

func (r *Runtime) Config() *config.Config {
	return r.cfg
}

func (r *Runtime) StartTime() time.Time {
	return r.startTime
}

func (r *Runtime) AudioCppPID() int {
	if r.backendMgr == nil {
		return 0
	}
	return r.backendMgr.PID()
}

func (r *Runtime) AudioCppState() backend.State {
	if r.backendMgr == nil {
		return backend.StateStopped
	}
	return r.backendMgr.State()
}

func (r *Runtime) IsAudioCppAlive() bool {
	if r.backendMgr == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.backendMgr.Health(ctx)
	return err == nil
}

func (r *Runtime) BackendManager() *backend.Manager {
	return r.backendMgr
}

func (r *Runtime) ReadyTime() time.Time {
	return r.readyTime
}

func (r *Runtime) ChildStartTime() time.Time {
	return r.childStartTime
}

func (r *Runtime) ShutdownTime() time.Time {
	return r.shutdownTime
}
