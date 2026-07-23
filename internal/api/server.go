package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/config"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/models"
	"github.com/liuxb99/audiocpp-runtime-go/internal/outputs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/runtime"
)

type Server struct {
	config       *config.Config
	router       *mux.Router
	audiocppCli  *audiocpp.Client
	process      *audiocpp.Process
	jobManager   *jobs.Manager
	modelReg     *models.Registry
	outputMgr    *outputs.Manager
	runtimeRef   *runtime.Runtime
	httpServer   *http.Server
	logger       *log.Logger
	startTime    time.Time
	shuttingDown atomic.Bool
}

func NewServer(cfg *config.Config, ac *audiocpp.Client, proc *audiocpp.Process, jm *jobs.Manager, mr *models.Registry, om *outputs.Manager, rt *runtime.Runtime) *Server {
	s := &Server{
		config:      cfg,
		audiocppCli: ac,
		process:     proc,
		jobManager:  jm,
		modelReg:    mr,
		outputMgr:   om,
		runtimeRef:  rt,
		logger:      log.Default(),
		startTime:   time.Now(),
	}
	s.router = mux.NewRouter()
	s.registerRoutes()
	return s
}

func (s *Server) Router() *mux.Router {
	return s.router
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	s.logger.Printf("API server starting on %s", addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	s.logger.Printf("API server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// SetShuttingDown marks the server as shutting down, causing the
// shuttingDownMiddleware to reject new requests with 503.
func (s *Server) SetShuttingDown() {
	s.shuttingDown.Store(true)
}
