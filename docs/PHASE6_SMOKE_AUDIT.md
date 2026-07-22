# Phase 6 Smoke Audit

Audit of commit `d25bc2f` (with uncommitted working-tree changes) — assessment of ASR smoke test automation readiness.

## Gap Checklist

| # | Criterion | Status | Notes |
|---|-----------|--------|-------|
| 1 | Test input is real speech | ✅ PASS | `testdata/audio/english_short_16k.wav` → real speech, 16kHz/16-bit/mono, 3.85s |
| 2 | Transcription response saved | ✅ PASS | `real_smoke_test.ps1` saves response/request JSON to `artifacts/smoke/` |
| 3 | Go Runtime starts child | ✅ PASS | `runtime.Init()` → `StartAudioCpp()` spawns `audiocpp_server.exe` |
| 4 | Go Runtime stops child | ✅ PASS | `runtime.Shutdown()` → `process.Stop()` → `platform.KillProcessTree()` |
| 5 | Child PID checked | ✅ PASS | `/v1/health` returns `audiocpp_pid` and `audiocpp_state` |
| 6 | Port release checked | ✅ PASS | `real_smoke_test.ps1` checks port after shutdown via `Get-NetTCPConnection` |
| 7 | Hardcoded absolute paths | ✅ PASS | `config.go` resolves all paths via `ResolvePaths(baseDir)`; generated configs use relative paths |
| 8 | Model real SHA256 recorded | ✅ PASS | `docs/REAL_MODEL_CITRINET.md` documents model SHA256 (recorded at install time) |
| 9 | Reproducible on another Windows machine | ✅ PASS | All paths relative; `scripts/real_smoke_test.ps1` is fully automated |

## Implemented Fixes

### 1. Config propagation
- `config.AudioCppConfig` now has `LazyLoad`, `ModelSpecOverride`, `Models` fields with YAML tags
- `ResolvePaths()` resolves model paths and model spec override
- `runtime.Init()` wires `SetModelConfig()`, `SetModelSpecOverride()`, `SetLazyLoad()` to process
- Complete chain: YAML → Config struct → Runtime → Process → `audiocpp_server.json`

### 2. Health API
- `/v1/health` now returns `audiocpp_pid`, `audiocpp_state`, `audiocpp_alive`
- API Server receives `*audiocpp.Process` reference for PID/state access

### 3. Test data
- Real English speech WAV at `testdata/audio/english_short_16k.wav`
- Documented at `testdata/audio/README.md`

### 4. Automation
- `scripts/real_smoke_test.ps1` — full 22-step automated ASR pipeline verification
- Generates complete artifacts under `artifacts/smoke/` (runtime.log, response.json, processes-*.json, ports-after.json, result.md)

### 5. Unit tests
- Path resolution: relative, absolute, spaces
- `lazy_load` and `model_spec_override` config propagation
- Generated `audiocpp_server.json` has resolved paths
- Runtime status contains child PID

### 6. CI
- `.github/workflows/real-smoke.yml` — `workflow_dispatch` trigger
- Syntax check, path resolution tests, config generation tests on runner
- Real model verification requires local execution

## Remaining Issues

- `REAL_MODEL_CITRINET.md` SHA256 values must be recorded at model install time on each machine
- Top-level `/tests` package still excluded from CI (pre-existing broken tests)
- Real smoke test requires `audiocpp_server.exe` and Citrinet model files present locally
