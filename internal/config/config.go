package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type AudioCppConfig struct {
	ServerPath         string `yaml:"server_path"`
	CliPath            string `yaml:"cli_path"`
	WorkingDir         string `yaml:"working_dir"`
	Backend            string `yaml:"backend"`
	Device             int    `yaml:"device"`
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	StartupTimeoutSec  int    `yaml:"startup_timeout_seconds"`
	RequestTimeoutSec  int    `yaml:"request_timeout_seconds"`
	AutoRestart        bool   `yaml:"auto_restart"`
	MaxRestartAttempts int    `yaml:"max_restart_attempts"`
}

type StorageConfig struct {
	SqlitePath string `yaml:"sqlite_path"`
}

type ModelsConfig struct {
	RootDir      string `yaml:"root_dir"`
	RegistryPath string `yaml:"registry_path"`
}

type OutputsConfig struct {
	RootDir    string `yaml:"root_dir"`
	RetainDays int    `yaml:"retain_days"`
}

type JobsConfig struct {
	Workers   int `yaml:"workers"`
	QueueSize int `yaml:"queue_size"`
}

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	AudioCpp AudioCppConfig `yaml:"audiocpp"`
	Storage  StorageConfig  `yaml:"storage"`
	Models   ModelsConfig   `yaml:"models"`
	Outputs  OutputsConfig  `yaml:"outputs"`
	Jobs     JobsConfig     `yaml:"jobs"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 8091,
		},
		AudioCpp: AudioCppConfig{
			ServerPath:         "runtime/audio.cpp/bin/audiocpp_server.exe",
			CliPath:            "runtime/audio.cpp/bin/audiocpp_cli.exe",
			WorkingDir:         "runtime/audio.cpp",
			Backend:            "cuda",
			Device:             0,
			Host:               "127.0.0.1",
			Port:               8092,
			StartupTimeoutSec:  120,
			RequestTimeoutSec:  600,
			AutoRestart:        true,
			MaxRestartAttempts: 5,
		},
		Storage: StorageConfig{
			SqlitePath: "data/runtime.db",
		},
		Models: ModelsConfig{
			RootDir:      "models",
			RegistryPath: "data/models.json",
		},
		Outputs: OutputsConfig{
			RootDir:    "outputs",
			RetainDays: 30,
		},
		Jobs: JobsConfig{
			Workers:   1,
			QueueSize: 100,
		},
	}
}

type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config validation failed (%d errors): %s",
		len(e.Errors), strings.Join(e.Errors, "; "))
}

func (c *Config) Validate(baseDir string) error {
	var errs []string

	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errs = append(errs, "server.port must be between 1 and 65535")
	}
	if c.AudioCpp.Port <= 0 || c.AudioCpp.Port > 65535 {
		errs = append(errs, "audiocpp.port must be between 1 and 65535")
	}
	if c.Server.Port == c.AudioCpp.Port {
		errs = append(errs, "server.port and audiocpp.port must be different")
	}
	if c.AudioCpp.StartupTimeoutSec < 10 {
		errs = append(errs, "audiocpp.startup_timeout_seconds must be >= 10")
	}
	if c.AudioCpp.RequestTimeoutSec < 10 {
		errs = append(errs, "audiocpp.request_timeout_seconds must be >= 10")
	}
	if c.AudioCpp.MaxRestartAttempts < 0 {
		errs = append(errs, "audiocpp.max_restart_attempts must be >= 0")
	}
	backend := strings.ToLower(c.AudioCpp.Backend)
	switch backend {
	case "cpu", "cuda", "vulkan", "metal", "best":
	default:
		errs = append(errs, fmt.Sprintf("audiocpp.backend must be cpu|cuda|vulkan|metal|best, got %q", c.AudioCpp.Backend))
	}
	if c.Jobs.Workers < 1 {
		errs = append(errs, "jobs.workers must be >= 1")
	}
	if c.Jobs.QueueSize < 1 {
		errs = append(errs, "jobs.queue_size must be >= 1")
	}
	if c.Outputs.RetainDays < 0 {
		errs = append(errs, "outputs.retain_days must be >= 0")
	}
	if c.Storage.SqlitePath == "" {
		errs = append(errs, "storage.sqlite_path must not be empty")
	}
	if _, err := os.Stat(filepath.Join(baseDir, c.AudioCpp.ServerPath)); err != nil {
		errs = append(errs, fmt.Sprintf("audiocpp.server_path not found: %s (resolved: %s)",
			c.AudioCpp.ServerPath, filepath.Join(baseDir, c.AudioCpp.ServerPath)))
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

func (c *Config) ResolvePaths(baseDir string) {
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(baseDir, p)
	}
	c.AudioCpp.ServerPath = resolve(c.AudioCpp.ServerPath)
	c.AudioCpp.CliPath = resolve(c.AudioCpp.CliPath)
	c.AudioCpp.WorkingDir = resolve(c.AudioCpp.WorkingDir)
	c.Storage.SqlitePath = resolve(c.Storage.SqlitePath)
	c.Models.RootDir = resolve(c.Models.RootDir)
	c.Models.RegistryPath = resolve(c.Models.RegistryPath)
	c.Outputs.RootDir = resolve(c.Outputs.RootDir)
}
