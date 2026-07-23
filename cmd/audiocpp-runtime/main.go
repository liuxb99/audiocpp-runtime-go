package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/liuxb99/audiocpp-runtime-go/internal/api"
	"github.com/liuxb99/audiocpp-runtime-go/internal/config"
	"github.com/liuxb99/audiocpp-runtime-go/internal/runtime"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	baseDir, _ := os.Getwd()

	cfg := config.DefaultConfig()
	if data, err := os.ReadFile(*configPath); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			log.Fatalf("failed to parse config: %v", err)
		}
	} else {
		log.Printf("no config file at %s, using defaults", *configPath)
	}
	cfg.ResolvePaths(baseDir)

	if err := cfg.Validate(baseDir); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}

	rt := runtime.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rt.Init(ctx); err != nil {
		log.Fatalf("runtime init failed: %v", err)
	}

	if err := rt.StartAudioCpp(ctx); err != nil {
		log.Fatalf("start audiocpp failed: %v", err)
	}

	rt.StartWorkers(cfg.Jobs.Workers)

	apiServer := api.NewServer(cfg, rt.Client(), rt.Process(), rt.JobManager(), rt.ModelRegistry(), rt.OutputManager(), rt)

	go func() {
		if err := apiServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	log.Printf("audiocpp-runtime started on %s:%d", cfg.Server.Host, cfg.Server.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Wait for either OS signal or API shutdown request
	select {
	case <-quit:
		log.Println("shutting down (signal)...")

		// Mark server as shutting down before we begin the shutdown sequence
		apiServer.SetShuttingDown()

		// Use Runtime.Shutdown() which handles graceful child stop, force kill,
		// state saving, cleanup, and database closing
		result := rt.Shutdown(context.Background())
		log.Printf("[main] shutdown result: request_accepted=%v graceful_exited=%v force_kill=%v runtime_exited=%v child_exited=%v",
			result.RequestAccepted, result.GracefulExited, result.ForceKillUsed,
			result.RuntimeExited, result.ChildExited)
		if schedule := rt.LastShutdownSchedule(); schedule != nil {
			log.Printf("[main] shutdown schedule:\n%s", schedule.Summary())
		}

		// Then stop the HTTP server
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := apiServer.Stop(shutdownCtx); err != nil {
			log.Printf("api server shutdown error: %v", err)
		}

	case <-apiServer.ShutdownRequested():
		// API handler already called rt.Shutdown() and closed HTTP server.
		// Just wait briefly then exit the process naturally.
		log.Println("shutting down (API request)...")
		time.Sleep(200 * time.Millisecond)
	}

	log.Println("audiocpp-runtime exited")
}
