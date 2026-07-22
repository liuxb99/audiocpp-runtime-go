package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected Server.Host 127.0.0.1, got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 8091 {
		t.Errorf("expected Server.Port 8091, got %d", cfg.Server.Port)
	}

	if cfg.AudioCpp.ServerPath != "runtime/audio.cpp/bin/audiocpp_server.exe" {
		t.Errorf("unexpected AudioCpp.ServerPath: %q", cfg.AudioCpp.ServerPath)
	}
	if cfg.AudioCpp.CliPath != "runtime/audio.cpp/bin/audiocpp_cli.exe" {
		t.Errorf("unexpected AudioCpp.CliPath: %q", cfg.AudioCpp.CliPath)
	}
	if cfg.AudioCpp.WorkingDir != "runtime/audio.cpp" {
		t.Errorf("unexpected AudioCpp.WorkingDir: %q", cfg.AudioCpp.WorkingDir)
	}
	if cfg.AudioCpp.Backend != "cuda" {
		t.Errorf("expected AudioCpp.Backend cuda, got %q", cfg.AudioCpp.Backend)
	}
	if cfg.AudioCpp.Host != "127.0.0.1" {
		t.Errorf("expected AudioCpp.Host 127.0.0.1, got %q", cfg.AudioCpp.Host)
	}
	if cfg.AudioCpp.Port != 8092 {
		t.Errorf("expected AudioCpp.Port 8092, got %d", cfg.AudioCpp.Port)
	}
	if cfg.AudioCpp.StartupTimeoutSec != 120 {
		t.Errorf("expected AudioCpp.StartupTimeoutSec 120, got %d", cfg.AudioCpp.StartupTimeoutSec)
	}
	if cfg.AudioCpp.RequestTimeoutSec != 600 {
		t.Errorf("expected AudioCpp.RequestTimeoutSec 600, got %d", cfg.AudioCpp.RequestTimeoutSec)
	}
	if !cfg.AudioCpp.AutoRestart {
		t.Error("expected AudioCpp.AutoRestart true")
	}
	if cfg.AudioCpp.MaxRestartAttempts != 5 {
		t.Errorf("expected AudioCpp.MaxRestartAttempts 5, got %d", cfg.AudioCpp.MaxRestartAttempts)
	}
	if cfg.AudioCpp.Threads != 1 {
		t.Errorf("expected AudioCpp.Threads 1, got %d", cfg.AudioCpp.Threads)
	}

	if cfg.Storage.SqlitePath != "data/runtime.db" {
		t.Errorf("unexpected Storage.SqlitePath: %q", cfg.Storage.SqlitePath)
	}
	if cfg.Models.RootDir != "models" {
		t.Errorf("unexpected Models.RootDir: %q", cfg.Models.RootDir)
	}
	if cfg.Models.RegistryPath != "data/models.json" {
		t.Errorf("unexpected Models.RegistryPath: %q", cfg.Models.RegistryPath)
	}
	if cfg.Outputs.RootDir != "outputs" {
		t.Errorf("unexpected Outputs.RootDir: %q", cfg.Outputs.RootDir)
	}
	if cfg.Outputs.RetainDays != 30 {
		t.Errorf("expected Outputs.RetainDays 30, got %d", cfg.Outputs.RetainDays)
	}
	if cfg.Jobs.Workers != 1 {
		t.Errorf("expected Jobs.Workers 1, got %d", cfg.Jobs.Workers)
	}
	if cfg.Jobs.QueueSize != 100 {
		t.Errorf("expected Jobs.QueueSize 100, got %d", cfg.Jobs.QueueSize)
	}
}

func createValidConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "runtime", "audio.cpp", "bin", "audiocpp_server.exe")
	if err := os.MkdirAll(filepath.Dir(serverPath), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	return dir
}

func TestValidate_Valid(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()

	if err := cfg.Validate(baseDir); err != nil {
		t.Errorf("expected valid config to pass, got: %v", err)
	}
}

func TestValidate_InvalidServerPort(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.Server.Port = 0

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for port 0")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == "server.port must be between 1 and 65535" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected server port error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_InvalidAudioCppPort(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.AudioCpp.Port = 99999

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for invalid audiocpp port")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == "audiocpp.port must be between 1 and 65535" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected audiocpp port error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_SamePorts(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.AudioCpp.Port = cfg.Server.Port

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for same ports")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == "server.port and audiocpp.port must be different" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected same-port error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_EmptySqlitePath(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.Storage.SqlitePath = ""

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for empty sqlite_path")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == "storage.sqlite_path must not be empty" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected sqlite_path error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_InvalidBackend(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.AudioCpp.Backend = "rocm"

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for invalid backend")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == `audiocpp.backend must be cpu|cuda|vulkan|metal|best, got "rocm"` {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected backend error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_InvalidWorkers(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.Jobs.Workers = 0

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for workers 0")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == "jobs.workers must be >= 1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected workers error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_InvalidQueueSize(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.Jobs.QueueSize = 0

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for queue_size 0")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == "jobs.queue_size must be >= 1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected queue_size error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_InvalidStartupTimeout(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.AudioCpp.StartupTimeoutSec = 5

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for startup timeout < 10")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == "audiocpp.startup_timeout_seconds must be >= 10" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected startup timeout error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_InvalidRetainDays(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.Outputs.RetainDays = -1

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for negative retain_days")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == "outputs.retain_days must be >= 0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected retain_days error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_MissingServerPath(t *testing.T) {
	baseDir := t.TempDir()
	cfg := DefaultConfig()

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for missing server_path")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == `audiocpp.server_path not found: runtime/audio.cpp/bin/audiocpp_server.exe (resolved: `+filepath.Join(baseDir, `runtime/audio.cpp/bin/audiocpp_server.exe`)+`)` {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected server_path error in validation errors: %v", ve.Errors)
	}
}

func TestValidate_InvalidMaxRestartAttempts(t *testing.T) {
	baseDir := createValidConfigDir(t)
	cfg := DefaultConfig()
	cfg.AudioCpp.MaxRestartAttempts = -1

	err := cfg.Validate(baseDir)
	if err == nil {
		t.Fatal("expected validation error for negative max_restart_attempts")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e == "audiocpp.max_restart_attempts must be >= 0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected max_restart_attempts error in validation errors: %v", ve.Errors)
	}
}

func TestResolvePaths_Relative(t *testing.T) {
	cfg := DefaultConfig()
	baseDir := "/base/dir"

	cfg.ResolvePaths(baseDir)

	expected := func(p string) string {
		return filepath.Join(baseDir, p)
	}

	if cfg.AudioCpp.ServerPath != expected("runtime/audio.cpp/bin/audiocpp_server.exe") {
		t.Errorf("ServerPath: got %q, want %q", cfg.AudioCpp.ServerPath, expected("runtime/audio.cpp/bin/audiocpp_server.exe"))
	}
	if cfg.AudioCpp.CliPath != expected("runtime/audio.cpp/bin/audiocpp_cli.exe") {
		t.Errorf("CliPath: got %q, want %q", cfg.AudioCpp.CliPath, expected("runtime/audio.cpp/bin/audiocpp_cli.exe"))
	}
	if cfg.AudioCpp.WorkingDir != expected("runtime/audio.cpp") {
		t.Errorf("WorkingDir: got %q, want %q", cfg.AudioCpp.WorkingDir, expected("runtime/audio.cpp"))
	}
	if cfg.Storage.SqlitePath != expected("data/runtime.db") {
		t.Errorf("SqlitePath: got %q, want %q", cfg.Storage.SqlitePath, expected("data/runtime.db"))
	}
	if cfg.Models.RootDir != expected("models") {
		t.Errorf("RootDir: got %q, want %q", cfg.Models.RootDir, expected("models"))
	}
	if cfg.Models.RegistryPath != expected("data/models.json") {
		t.Errorf("RegistryPath: got %q, want %q", cfg.Models.RegistryPath, expected("data/models.json"))
	}
	if cfg.Outputs.RootDir != expected("outputs") {
		t.Errorf("Outputs.RootDir: got %q, want %q", cfg.Outputs.RootDir, expected("outputs"))
	}
}

func TestResolvePaths_Absolute(t *testing.T) {
	absServerPath := filepath.Join("C:", "absolute", "server.exe")
	absModelsDir := filepath.Join("C:", "absolute", "models")
	if !filepath.IsAbs(absServerPath) {
		absServerPath = filepath.Join(t.TempDir(), "server.exe")
		absModelsDir = filepath.Join(t.TempDir(), "models")
	}

	cfg := DefaultConfig()
	cfg.AudioCpp.ServerPath = absServerPath
	cfg.Models.RootDir = absModelsDir
	baseDir := "C:\\some\\base"

	cfg.ResolvePaths(baseDir)

	if cfg.AudioCpp.ServerPath != absServerPath {
		t.Errorf("expected ServerPath unchanged, got %q", cfg.AudioCpp.ServerPath)
	}
	if cfg.Models.RootDir != absModelsDir {
		t.Errorf("expected RootDir unchanged, got %q", cfg.Models.RootDir)
	}
}

func TestResolveRelativeModelPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AudioCpp.Models = []ModelConfig{
		{ID: "test-model", Path: "models/test", Family: "citrinet_asr"},
	}
	baseDir := "/repo/root"
	cfg.ResolvePaths(baseDir)

	want := filepath.Join(baseDir, "models/test")
	if cfg.AudioCpp.Models[0].Path != want {
		t.Errorf("expected model path %q, got %q", want, cfg.AudioCpp.Models[0].Path)
	}
}

func TestResolveAbsoluteModelPath(t *testing.T) {
	absPath := filepath.Join("C:", "absolute", "models", "test")
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(t.TempDir(), "models", "test")
	}

	cfg := DefaultConfig()
	cfg.AudioCpp.Models = []ModelConfig{
		{ID: "test-model", Path: absPath, Family: "citrinet_asr"},
	}
	cfg.ResolvePaths("/some/base")

	if cfg.AudioCpp.Models[0].Path != absPath {
		t.Errorf("expected absolute path unchanged %q, got %q", absPath, cfg.AudioCpp.Models[0].Path)
	}
}

func TestResolvePathWithSpaces(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AudioCpp.ModelSpecOverride = "audio.cpp/model specs"
	cfg.AudioCpp.Models = []ModelConfig{
		{ID: "test", Path: "models/my models/citrinet", Family: "citrinet_asr"},
	}
	baseDir := "C:/repo root"
	cfg.ResolvePaths(baseDir)

	wantModelSpec := filepath.Join(baseDir, "audio.cpp/model specs")
	if cfg.AudioCpp.ModelSpecOverride != wantModelSpec {
		t.Errorf("expected model_spec_override %q, got %q", wantModelSpec, cfg.AudioCpp.ModelSpecOverride)
	}

	wantModelPath := filepath.Join(baseDir, "models/my models/citrinet")
	if cfg.AudioCpp.Models[0].Path != wantModelPath {
		t.Errorf("expected model path %q, got %q", wantModelPath, cfg.AudioCpp.Models[0].Path)
	}
}

func TestLazyLoadDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AudioCpp.LazyLoad {
		t.Error("expected LazyLoad to be false by default")
	}
}

func TestModelSpecOverrideDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AudioCpp.ModelSpecOverride != "" {
		t.Errorf("expected ModelSpecOverride to be empty, got %q", cfg.AudioCpp.ModelSpecOverride)
	}
}

func TestModelsDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AudioCpp.Models != nil {
		t.Errorf("expected Models to be nil, got %v", cfg.AudioCpp.Models)
	}
}

func TestResolveModelSpecOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AudioCpp.ModelSpecOverride = "./model_specs"
	baseDir := "/base"
	cfg.ResolvePaths(baseDir)

	want := filepath.Join(baseDir, "./model_specs")
	got := cfg.AudioCpp.ModelSpecOverride
	if got != want {
		t.Errorf("expected ModelSpecOverride %q, got %q", want, got)
	}
}

func TestResolveModelSpecOverrideEmpty(t *testing.T) {
	cfg := DefaultConfig()
	baseDir := "/base"
	cfg.ResolvePaths(baseDir)

	if cfg.AudioCpp.ModelSpecOverride != "" {
		t.Errorf("expected empty ModelSpecOverride, got %q", cfg.AudioCpp.ModelSpecOverride)
	}
}
