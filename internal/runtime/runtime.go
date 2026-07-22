package runtime

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/config"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/models"
	"github.com/liuxb99/audiocpp-runtime-go/internal/outputs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
)

type Runtime struct {
	mu     sync.RWMutex
	cfg    *config.Config
	proc   *audiocpp.Process
	cli    *audiocpp.CLIExecutor
	client *audiocpp.Client

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
	started     bool
}

func New(cfg *config.Config) *Runtime {
	return &Runtime{
		cfg:       cfg,
		startTime: time.Now(),
	}
}

func (r *Runtime) Init(ctx context.Context) error {
	log.Printf("[runtime] initializing")

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
	r.jobMgr = jobs.NewManager(r.jobRepo)

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

	log.Printf("[runtime] initialization complete")
	return nil
}

func (r *Runtime) StartAudioCpp(ctx context.Context) error {
	if r.proc == nil {
		return fmt.Errorf("process not initialized")
	}

	log.Printf("[runtime] starting audiocpp server")
	if err := r.proc.Start(r.lifetimeCtx); err != nil {
		return fmt.Errorf("start audiocpp server: %w", err)
	}

	readyCtx, cancel := context.WithTimeout(r.lifetimeCtx,
		time.Duration(r.cfg.AudioCpp.StartupTimeoutSec)*time.Second)
	defer cancel()

	if err := r.proc.WaitForReady(readyCtx,
		time.Duration(r.cfg.AudioCpp.StartupTimeoutSec)*time.Second); err != nil {
		return fmt.Errorf("audiocpp server not ready: %w", err)
	}

	log.Printf("[runtime] audiocpp server is ready")

	if err := r.modelReg.Refresh(ctx, r.client); err != nil {
		log.Printf("[runtime] warning: model registry refresh failed: %v", err)
	}

	return nil
}

func (r *Runtime) StartWorkers(count int) {
	r.workerPool = jobs.NewWorkerPool(r.jobMgr, r.client, count)
	r.workerPool.Start()
	r.started = true
}

func (r *Runtime) StopWorkers() {
	if r.workerPool != nil {
		r.workerPool.Stop()
	}
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	log.Printf("[runtime] shutting down")

	if r.workerPool != nil {
		r.workerPool.Stop()
	}

	if r.proc != nil {
		if err := r.proc.Stop(); err != nil {
			log.Printf("[runtime] error stopping audiocpp: %v", err)
		}
	}

	r.lifetimeCnl()

	if r.modelReg != nil {
		if err := r.modelReg.Save(); err != nil {
			log.Printf("[runtime] error saving model registry: %v", err)
		}
	}

	if r.outputMgr != nil {
		if _, err := r.outputMgr.Cleanup(ctx); err != nil {
			log.Printf("[runtime] error cleaning outputs: %v", err)
		}
	}

	if r.db != nil {
		if err := r.db.Close(); err != nil {
			log.Printf("[runtime] error closing database: %v", err)
		}
	}

	log.Printf("[runtime] shutdown complete")
	return nil
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
	if r.proc == nil {
		return 0
	}
	return r.proc.Pid()
}

func (r *Runtime) AudioCppState() audiocpp.ProcessState {
	if r.proc == nil {
		return audiocpp.StateStopped
	}
	return r.proc.State()
}

func (r *Runtime) IsAudioCppAlive() bool {
	if r.client == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.client.Health(ctx)
	return err == nil
}
