# Development Guide

## Prerequisites

- Go 1.22+
- Git
- Visual Studio 2022 (for Windows CUDA builds of audio.cpp)
- CUDA Toolkit 12+ (optional, for GPU support)

## Setup

```bash
git clone --recursive https://github.com/liuxb99/audiocpp-runtime-go
cd audiocpp-runtime-go
```

## Build

```bash
# Build both binaries
.\build.bat

# Or individually
go build -o bin\audiocpp-runtime.exe .\cmd\audiocpp-runtime\
go build -o bin\audiocppctl.exe .\cmd\audiocppctl\
```

## Test

```bash
go test -v -count=1 ./...
```

## Run

```bash
# With default config
.\scripts\run.bat

# With custom config
bin\audiocpp-runtime.exe --config configs\myconfig.yaml
```

## CLI Usage

```bash
.\bin\audiocppctl.exe health
.\bin\audiocppctl.exe models
.\bin\audiocppctl.exe tts --model pocket-tts --text "Hello" --out output.wav
.\bin\audiocppctl.exe asr --model whisper --audio input.wav
```

## Project Structure

```
├── cmd/
│   ├── audiocpp-runtime/    # Main runtime binary
│   └── audiocppctl/         # CLI tool
├── internal/
│   ├── api/                 # HTTP API server and handlers
│   ├── audiocpp/            # AudioC++ client, process, CLI, errors
│   ├── config/              # Configuration loading and validation
│   ├── jobs/                # Job system (model, queue, manager, worker pool)
│   ├── models/              # Model registry and manifests
│   ├── outputs/             # Output file management
│   ├── platform/            # Platform-specific code (Windows/Unix)
│   └── storage/             # SQLite database and repositories
├── docs/                    # Documentation
├── configs/                 # Configuration examples
├── scripts/                 # Build, test, and smoke test scripts
├── tests/                   # Integration tests
├── web/                     # Web UI (HTML/JS dashboard)
└── migrations/              # SQL migration files
```

## Adding New Endpoints

1. Add handler method to `internal/api/` (e.g., `handlers_*.go`)
2. Register route in `internal/api/routes.go`
3. Add client method to `internal/audiocpp/client.go` if proxying to audiocpp_server
4. Add tests in `tests/integration_test.go`

## Code Conventions

- Use `gofmt` for formatting
- Follow standard Go project layout
- Use table-driven tests where appropriate
- Log via `log.Printf` with component prefix (`[runtime]`, `[audiocpp]`, `[jobs]`)

## Config File

See `configs/config.example.yaml` for all available options.
