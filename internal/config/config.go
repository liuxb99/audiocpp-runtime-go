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

type ModelConfig struct {
	ID                 string                 `yaml:"id"`
	Path               string                 `yaml:"path"`
	Family             string                 `yaml:"family"`
	Task               string                 `yaml:"task"`
	Mode               string                 `yaml:"mode"`
	Lazy               bool                   `yaml:"lazy"`
	VoicePresets       map[string]VoicePreset `yaml:"voice_presets"`
	DefaultVoicePreset interface{}            `yaml:"default_voice_preset"`
	LoadOptions        map[string]string      `yaml:"load_options"`
	SessionOptions     map[string]string      `yaml:"session_options"`
}

type VoicePreset struct {
	VoiceID       string `yaml:"voice_id"`
	VoiceRef      string `yaml:"voice_ref"`
	ReferenceText string `yaml:"reference_text"`
}

type AudioCppConfig struct {
	ServerPath         string        `yaml:"server_path"`
	CliPath            string        `yaml:"cli_path"`
	WorkingDir         string        `yaml:"working_dir"`
	Backend            string        `yaml:"backend"`
	Device             int           `yaml:"device"`
	Threads            int           `yaml:"threads"`
	Host               string        `yaml:"host"`
	Port               int           `yaml:"port"`
	StartupTimeoutSec  int           `yaml:"startup_timeout_seconds"`
	RequestTimeoutSec  int           `yaml:"request_timeout_seconds"`
	AutoRestart        bool          `yaml:"auto_restart"`
	MaxRestartAttempts int           `yaml:"max_restart_attempts"`
	LazyLoad           bool          `yaml:"lazy_load"`
	ModelSpecOverride  string        `yaml:"model_spec_override"`
	Models             []ModelConfig `yaml:"models"`
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

type BackendConfig struct {
	Type string `yaml:"type"`
}

type JobsConfig struct {
	Workers               int `yaml:"workers"`
	QueueSize             int `yaml:"queue_size"`
	DefaultTimeoutSeconds int `yaml:"default_timeout_seconds"`
	MaxAttempts           int `yaml:"max_attempts"`
	RetryInitialDelayMs   int `yaml:"retry_initial_delay_ms"`
	RetryMaxDelayMs       int `yaml:"retry_max_delay_ms"`
	QueueCapacity         int `yaml:"queue_capacity"`
}

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Backend  BackendConfig  `yaml:"backend"`
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
		Backend: BackendConfig{
			Type: "audiocpp",
		},
		AudioCpp: AudioCppConfig{
			ServerPath:         "runtime/audio.cpp/bin/audiocpp_server.exe",
			CliPath:            "runtime/audio.cpp/bin/audiocpp_cli.exe",
			WorkingDir:         "runtime/audio.cpp",
			Backend:            "cuda",
			Device:             0,
			Threads:            1,
			Host:               "127.0.0.1",
			Port:               8092,
			StartupTimeoutSec:  120,
			RequestTimeoutSec:  600,
			AutoRestart:        true,
			MaxRestartAttempts: 5,
			LazyLoad:           false,
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
			Workers:               1,
			QueueSize:             100,
			DefaultTimeoutSeconds: 300,
			MaxAttempts:           3,
			RetryInitialDelayMs:   500,
			RetryMaxDelayMs:       5000,
			QueueCapacity:         1000,
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
	if c.Jobs.DefaultTimeoutSeconds < 10 {
		errs = append(errs, "jobs.default_timeout_seconds must be >= 10")
	}
	if c.Jobs.MaxAttempts < 1 {
		errs = append(errs, "jobs.max_attempts must be >= 1")
	}
	if c.Jobs.RetryInitialDelayMs < 100 {
		errs = append(errs, "jobs.retry_initial_delay_ms must be >= 100")
	}
	if c.Jobs.RetryMaxDelayMs < c.Jobs.RetryInitialDelayMs {
		errs = append(errs, "jobs.retry_max_delay_ms must be >= retry_initial_delay_ms")
	}
	if c.Jobs.QueueCapacity < 1 {
		errs = append(errs, "jobs.queue_capacity must be >= 1")
	}
	if c.Outputs.RetainDays < 0 {
		errs = append(errs, "outputs.retain_days must be >= 0")
	}
	if c.Storage.SqlitePath == "" {
		errs = append(errs, "storage.sqlite_path must not be empty")
	}
	if c.AudioCpp.ServerPath != "" {
		resolvedServerPath := c.AudioCpp.ServerPath
		if !filepath.IsAbs(resolvedServerPath) {
			resolvedServerPath = filepath.Join(baseDir, resolvedServerPath)
		}
		if _, err := os.Stat(resolvedServerPath); err != nil {
			errs = append(errs, fmt.Sprintf("audiocpp.server_path not found: %s (resolved: %s)",
				c.AudioCpp.ServerPath, resolvedServerPath))
		}
	}

	if c.Backend.Type != "" {
		bt := strings.TrimSpace(c.Backend.Type)
		if bt == "" {
			errs = append(errs, "backend.type must not be only whitespace")
		}
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// BackendType 傳回後端類型，若未設定則回傳預設值 "audiocpp"
func (c *Config) BackendType() string {
	if c.Backend.Type == "" {
		return "audiocpp"
	}
	return c.Backend.Type
}

func (c *Config) ResolvePaths(baseDir string) {
	resolve := func(p string) string {
		if p == "" {
			return ""
		}
		// Normalize slashes for Windows IsAbs check
		p = filepath.FromSlash(p)
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(baseDir, p)
	}
	c.AudioCpp.ServerPath = resolve(c.AudioCpp.ServerPath)
	c.AudioCpp.CliPath = resolve(c.AudioCpp.CliPath)
	c.AudioCpp.WorkingDir = resolve(c.AudioCpp.WorkingDir)
	c.AudioCpp.ModelSpecOverride = resolve(c.AudioCpp.ModelSpecOverride)
	c.Storage.SqlitePath = resolve(c.Storage.SqlitePath)
	c.Models.RootDir = resolve(c.Models.RootDir)
	c.Models.RegistryPath = resolve(c.Models.RegistryPath)
	c.Outputs.RootDir = resolve(c.Outputs.RootDir)
	for i := range c.AudioCpp.Models {
		c.AudioCpp.Models[i].Path = resolve(c.AudioCpp.Models[i].Path)
	}
}
