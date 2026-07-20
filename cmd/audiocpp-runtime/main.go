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
	"github.com/liuxb99/audiocpp-runtime-go/internal/audiocpp"
	"github.com/liuxb99/audiocpp-runtime-go/internal/config"
	"github.com/liuxb99/audiocpp-runtime-go/internal/jobs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/models"
	"github.com/liuxb99/audiocpp-runtime-go/internal/outputs"
	"github.com/liuxb99/audiocpp-runtime-go/internal/storage"
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

	db, err := storage.NewDB(cfg.Storage.SqlitePath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.RunMigrations(); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	ac := audiocpp.NewClient(cfg.AudioCpp.Host, cfg.AudioCpp.Port,
		time.Duration(cfg.AudioCpp.RequestTimeoutSec)*time.Second)

	modelReg := models.NewRegistry(cfg.Models.RegistryPath)
	if err := modelReg.Load(); err != nil {
		log.Printf("warning: could not load model registry: %v", err)
	}

	jobRepo := storage.NewJobsRepository(db)
	outputRepo := storage.NewOutputsRepository(db)

	outputMgr := outputs.NewManager(cfg.Outputs.RootDir, cfg.Outputs.RetainDays, outputRepo)

	jobMgr := jobs.NewManager(jobRepo)
	workerPool := jobs.NewWorkerPool(jobMgr, ac, cfg.Jobs.Workers)
	workerPool.Start()

	apiServer := api.NewServer(cfg, ac, jobMgr, modelReg, outputMgr)

	go func() {
		if err := apiServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	log.Printf("audiocpp-runtime started on %s:%d", cfg.Server.Host, cfg.Server.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := apiServer.Stop(ctx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
}
