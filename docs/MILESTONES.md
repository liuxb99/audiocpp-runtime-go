# Project Milestones — audio.cpp-runtime-go

> **文件版本**: v1.0  
> **基準提交**: [`f009937`](https://github.com/liuxb99/audiocpp-runtime-go/commit/f009937)  
> **Go 版本**: go1.26.4 windows/amd64  
> **授權**: MIT  
> **更新日期**: 2026-07-23

---

## 目前完成 (Completed Phases)

### Phase 1 — 專案初始化與基本架構

| 交付物 | 說明 |
|--------|------|
| Go Module Scaffold | `github.com/liuxb99/audiocpp-runtime-go`，含 `.gitignore`、`build.bat` |
| 基本 Runtime 管理 | `internal/runtime/manager.go` — Runtime 生命週期管理、`config.go` — 基本配置 |
| 內建 Web Server | `internal/server/` — 基於 gorilla/mux 的 HTTP server，含 HTML template 引擎、檔案上傳處理 |
| Model Catalog | `internal/catalog/` — 靜態 JSON 模型目錄系統（後續被 Model Registry 取代） |
| Git Submodule | `audio.cpp`、`audio.cpp-webui` 作為 submodule 引用 |
| 核心依賴 | `github.com/gorilla/mux v1.8.1`、Go 1.22 → 1.25 → 1.26.4 |

### Phase 2 — API Server 與 Job System

| 交付物 | 說明 |
|--------|------|
| OpenAI-compatible HTTP API | 15+ 端點，含 `/v1/audio/speech`、`/v1/audio/transcriptions`、`/v1/health`、`/v1/models`、`/v1/shutdown` 等 |
| Job System | Priority Queue (heap-based)、Job Manager (CRUD + 持久化)、Worker Pool (多 worker 並行) |
| audio.cpp Client | HTTP client 封裝 audio.cpp server 所有端點；CLI Executor 作為 fallback |
| 跨平台 Process 管理 | `internal/platform/` — Windows (`CreateProcess` / `taskkill /T /F`) 與 Unix (`SIGTERM` / `SIGKILL`) |
| YAML 配置系統 | `internal/config/` — 完整 Config 結構體、路徑解析、配置驗證、單元測試 |
| Backend Capabilities 映射 | 靜態 task↔capabilities 雙向映射 (Go map) |

### Phase 3 — Model Registry 與 Storage

| 交付物 | 說明 |
|--------|------|
| Model Registry | `internal/models/` — Manifest (JSON)、Registry (載入/儲存/自動刷新)、能力索引、語言索引 |
| SQLite Storage | `internal/storage/` — 連線管理 (WAL 模式)、自動遷移系統 (`migrations/001_init.sql`)、Jobs/Outputs Repository |
| Output Manager | `internal/outputs/` — 音訊檔案儲存管理、MIME type 對應、保留期限自動清理 |
| 共用 API Types | `pkg/api/types.go` — `JobRequest`、`HealthResponse`、`TranscribeRequest`、`SpeechRequest` 等 |

### Phase 4 — Integration 與 Worker Pool

| 交付物 | 說明 |
|--------|------|
| Runtime Orchestration | `Init()` → `Start()` → `Shutdown()` 完整流程，元件初始化順序：Storage → Registry → Process → Client → Workers → API |
| Worker Pool | 可設定 worker 數量、dequeue → execute → save 循環、優雅關閉 drain |
| Integration Tests | `tests/integration_test.go` (API 端到端) + `tests/integration_db_test.go` (資料庫 CRUD) |
| 完整 API Types | `pkg/api/types.go` — 跨 package 共用型別定義 |

### Phase 5 — Web UI 與 CLI

| 交付物 | 說明 |
|--------|------|
| CLI Tool (`audiocppctl`) | `start` / `stop` / `status` / `jobs` / `models` / `health` 子命令 |
| Web UI | `web/index.html` — 單頁應用 (HTML+CSS+JS)，TTS/ASR 測試、Job 佇列檢視、Server 狀態監控 |
| 文件 | `API.md`、`ARCHITECTURE.md`、`DEVELOPMENT.md`、`UPSTREAM_ANALYSIS.md` (363 行) |
| Scripts | `build.bat`、`run.bat`、`smoke_test.bat`、`test.bat` |

### Phase 6 — Real ASR、Smoke Test、Runtime Stabilization

Phase 6 包含 5 個子階段 (6A~6E)，為正式發布版本的核心工作。

#### Phase 6A — Real ASR 端到端驗證

| 交付物 | 說明 |
|--------|------|
| 真實語音測試檔案 | `testdata/audio/english_short_16k.wav` (16kHz/16-bit/mono, 3.85 秒) |
| 真實 Citrinet ASR 模型支援 | 文件：`docs/REAL_MODEL_CITRINET.md`、`docs/REAL_AUDIOCPP_BUILD.md` |
| Config 傳遞鏈 | `lazy_load`、`model_spec_override`、`Models` 完整 YAML → Config → Runtime → Process → `audiocpp_server.json` |
| Health API 強化 | `/v1/health` 回傳 `audiocpp_pid`、`audiocpp_state`、`audiocpp_alive` |
| Automated Smoke Script | `scripts/real_smoke_test.ps1` (22 步驟完整 ASR 管線驗證) |
| CI Workflow | `.github/workflows/ci.yml` (自動化) + `.github/workflows/real-smoke.yml` (手動觸發) |
| Process Manager 強化 | Config JSON 自動生成、Process 單元測試 (376 行)、Unix `KillProcessTree` |

#### Phase 6B — 驗收漏洞修補

| 交付物 | 說明 |
|--------|------|
| CGO 依賴移除 | `modernc.org/sqlite` → `github.com/glebarez/go-sqlite` (純 Go) |
| Go 版本升級 | Go 1.22 → 1.25，CI 更新 |
| Smoke 驗證強化 | 內容匹配門檻 (≥5 words + ≥50%)、HTTP Status 參與 PASS、真正 SHA256 驗證、BOM/Emoji 修復 |
| Graceful Shutdown | 先嘗試正常停止 → 5 秒等待 → 最後 `taskkill /F` |
| Ready 判定強化 | `status==ok` + `audiocpp_alive==true` + `audiocpp_state==running` + `pid>0` |
| Process List 過濾 | 只保留 runtime/audiocpp 的 pid/alive，不提交整台電腦 process list |

#### Phase 6C — Final Evidence Cleanup

| 交付物 | 說明 |
|--------|------|
| Shutdown 語義修正 | 拆分為 3 欄位：`RequestAccepted` / `GracefulExited` / `ForceKillUsed` |
| Smoke Evidence 模型 | 3 欄位證據模型：`tested_source_commit` / `evidence_generated_at` / `evidence_commit` |
| Main process 退出通道 | `handleShutdown` 新增 channel 通知 main goroutine 退出 |
| Shutdown 測試 | `TestShutdownEndpointStopsRuntime`、`TestShutdownStopsAudioCppChild`、`TestShutdownDoesNotRequireForceKill`、`TestShutdownClosesStorage` |

#### Phase 6D — Runtime Stabilization

| 交付物 | 說明 |
|--------|------|
| Runtime State Machine | `Created` → `Initializing` → `Starting` → `Ready` → `Running` → `Stopping` → `Stopped`，單一路徑、禁止跳躍、禁止重複進入 |
| Shutdown Contract | 8 步驟合同：`RequestAccepted` → `StopWorkers` → `FlushQueue` → `StopChild` → `SaveState` → `CloseDB` → `StopHTTP` → `ExitMain` |
| Backend Contract Test | `internal/runtime/contract_test.go` (432 行)，8 項 contract 驗證 (Start/Ready/Health/Stop/ForceStop/PID/Version/ Capabilities) |
| Runtime Diagnostics | `GET /v1/runtime/diagnostics` — Startup Time、Ready Time、Child Start Time、Shutdown Time、Worker Count、Queue Length、Current State 等 |
| Runtime 測試 | `internal/runtime/runtime_test.go` (261 行) — Lifecycle test、State transition test、Diagnostics test |
| Backend 測試 | `internal/audiocpp/backend_test.go` (212 行) — Fake child process contract 驗證 |
| Process 測試強化 | `internal/audiocpp/process_test.go` (465 行) — Config generation、Model spec override、Lazy load、Path resolution |
| ADR 文件 | ADR-0001~0005 架構決策記錄 |

#### Phase 6E — Final Closure

| 交付物 | 說明 |
|--------|------|
| Backend Contract 補齊 | ForceStop Contract、Capabilities Contract、Version (Known Gap SKIP) |
| Diagnostics 驗證整合 | Diagnostics 檢查加入 smoke PASS 條件，儲存為 `artifacts/smoke/runtime_diagnostics.json` |
| TestGeneratedConfigJSON 修正 | 根因：`test.exe` → `testBinaryPath(t)`，production validation 保持嚴格 |
| 最終真實 Smoke | Source Commit `b799580` → 真實 audio.cpp + Citrinet + real WAV → **REAL_SMOKE_PASS** ✅ |
| Evidence 分離提交 | Source Commit (程式碼+測試) 與 Evidence Commit (僅 `artifacts/smoke/`) 分離 |

---

## 目前品質 (Quality Status)

### 建置與靜態分析

| 檢查 | 狀態 | 說明 |
|------|------|------|
| `go vet ./...` | ✅ PASS | 無任何 vet 警告 |
| `go build ./...` | ✅ PASS | 所有套件建置成功 |
| `go test ./...` | ✅ PASS | 7 個含測試的套件全部通過 |

### 測試統計

| 指標 | 數值 |
|------|------|
| 測試檔案總數 | 11 個 (`*_test.go`) |
| 含測試的套件 | 7 個 (api, audiocpp, config, jobs, models, runtime, tests) |
| 不含測試的套件 | 5 個 (cmd/audiocpp-runtime, cmd/audiocppctl, outputs, platform, storage, pkg/api) |
| 總 Go 原始碼行數 | ~9,225 行 |
| 測試套件總執行時間 | ~122 秒 |

### 已知測試限制

| 項目 | 狀態 | 原因 |
|------|------|------|
| **Race Test** (`go test -race ./...`) | ⛔ NOT RUN | Windows 環境缺少 gcc/cgo toolchain，無法啟用 `-race` |
| **Version Contract** (`TestBackendContract_Version`) | ⏭️ SKIP (Known Gap) | audio.cpp 上游未實作 `/v1/version` endpoint |
| **Top-level `/tests` 套件** | ⚠️ 存在但 CI 排除 | 既有測試問題，Phase 7 目標修復後納入 CI |

---

## 目前 Coverage (測試覆蓋)

### Smoke Test 狀態

| 項目 | 狀態 | 說明 |
|------|------|------|
| **REAL_SMOKE_PASS** | ✅ **通過** | 基於 Source Commit `b799580`，真實 audio.cpp binary + Citrinet 模型 |
| HTTP Status | ✅ 200 | ASR 請求成功 |
| 轉錄匹配 | ✅ 6/9 words (66.7%) | 門檻：≥5 words + ≥50% |
| Shutdown Request Accepted | ✅ True | API shutdown 成功接受 |
| Runtime Graceful Exit | ✅ True | Runtime 主程序自然退出 |
| External Force Kill | ❌ False | 未使用強制終止 (正確行為) |
| Diagnostics Passed | ✅ True | Diagnostics HTTP 200 + JSON 合法 + 關鍵欄位非空 |

### Backend Contract Tests 狀態

| Contract | 狀態 | 說明 |
|----------|------|------|
| TestBackendContract_Start | ✅ PASS | Fake child process start |
| TestBackendContract_Ready | ✅ PASS | Ready state detection |
| TestBackendContract_Health | ✅ PASS | Health endpoint query |
| TestBackendContract_Stop | ✅ PASS | Graceful stop |
| TestBackendContract_ForceStop | ✅ PASS | Force stop with child termination |
| TestBackendContract_PID | ✅ PASS | PID tracking |
| TestBackendContract_Capabilities | ✅ PASS | Capabilities non-empty, no duplicates |
| TestBackendContract_Version | ⏭️ SKIP | **Known Gap**: audio.cpp 無 `/v1/version` |

### Runtime Lifecycle Tests 狀態

| Test | 狀態 | 說明 |
|------|------|------|
| TestRuntimeLifecycle_FullCycle | ✅ PASS | 完整 Init → Start → Shutdown |
| TestShutdownContract_FullSequence | ✅ PASS | 8 步驟 shutdown contract |
| TestStateTransition_DoubleShutdown | ✅ PASS | Double shutdown idempotent |
| TestStateTransition_ShutdownBeforeStart | ✅ PASS | Shutdown before start |
| TestStateTransition_InvalidTransition | ✅ PASS | Invalid state transition rejection |
| TestDiagnostics_Collect | ✅ PASS | Diagnostics data collection |
| TestDiagnostics_AfterShutdown | ✅ PASS | Diagnostics after shutdown |
| TestBackend_StartupFailure | ✅ PASS | Backend startup failure handling |
| TestBackend_ReadyTimeout | ✅ PASS | Ready timeout handling |
| TestBackend_ChildCrash | ✅ PASS | Child process crash recovery |
| TestShutdown_Timeout | ✅ PASS | Shutdown timeout handling |
| TestRuntimeStartsAudioCpp | ✅ PASS | Runtime starts child process |
| TestRuntimeShutdownStopsAudioCpp | ✅ PASS | Shutdown stops child |
| TestRuntimeStartupFailureCleansChild | ✅ PASS | Startup failure cleans up child |
| TestRuntimeStatusContainsChildPID | ✅ PASS | Health status contains child PID |

### Shutdown Tests 狀態

| Test | 狀態 |
|------|------|
| TestShutdownEndpointStopsRuntime | ✅ PASS |
| TestShutdownStopsAudioCppChild | ✅ PASS |
| TestShutdownDoesNotRequireForceKill | ✅ PASS |
| TestShutdownClosesStorage | ✅ PASS |

---

## 目前 Smoke (Latest Real Smoke Result)

### 最終 Smoke 驗證 (Phase 6E Final Closure)

| 項目 | 結果 |
|------|------|
| **Verdict** | ✅ **REAL_SMOKE_PASS** |
| Source Commit | `b7995808bd32692990a0bd6c965ca438100c7e62` |
| Evidence Commit | `95050d9121cccc1f3733e4332efd82f316707b5f` |
| 執行時間 | 2026-07-23 22:01:52 |
| 總耗時 | 10,502 ms |
| HTTP Status | **200** |
| 語音檔案 | `testdata/audio/english_short_16k.wav` (16kHz WAV, 3.85s) |
| 模型 | Citrinet 256 (SHA256 已驗證) |
| audio.cpp Upstream SHA | `cd91110b39ad48cdb594d893687e9d2ae8ce0dbf` |

### 匹配結果

```
Expected: "The quick brown fox jumps over the lazy dog"
Got:      "the quickbrown fox jumps over the lazy blog"
Match:    6/9 words (66.7%)  ✅ (threshold: ≥5 words, ≥50%)
```

### Shutdown 品質

| 指標 | 值 | 說明 |
|------|----|------|
| Shutdown Request Accepted | ✅ True | API 成功接受 shutdown |
| Runtime Graceful Exit | ✅ True | 主程序自然退出 (無強制終止) |
| External Force Kill Used | ❌ False | ✅ **未使用** 外部強制終止 |
| Child Graceful Exit | ✅ True | Child 程序優雅退出 |
| Child Force Kill Used | ❌ False | Child 未使用強制終止 |
| Shutdown Duration | 6,734 ms | 含 5 秒 graceful 等待視窗 |

### Diagnostics 驗證

| 項目 | 結果 |
|------|------|
| HTTP 200 | ✅ |
| JSON 可解析 | ✅ |
| 關鍵欄位非空 | ✅ |
| 保存路徑 | `artifacts/smoke/runtime_diagnostics.json` |

---

## 目前限制 (Current Limitations)

### 技術債摘要 (15 項)

| 優先級 | 項目數 | 核心項目 |
|--------|--------|----------|
| **P0 (緊急)** | 1 | **TD-005**: 僅支援 audio.cpp 單一 Backend，無 Backend 抽象層 — 阻止多 Backend 支援 |
| **P1 (重要)** | 4 | **TD-002**: Child Process Graceful Shutdown 未實作；**TD-006**: GPU Backend 未實作；**TD-007**: Streaming Pipeline 未實作；**TD-012**: HTTPS/API 認證缺失 |
| **P2 (一般)** | 10 | Version Endpoint、Race Test、Capabilities 擴充、Batch Inference、Hot Reload、Model Cache、Lazy Load 衝突、結構化日誌、測試覆蓋率、持續 Metrics |

完整清單請參閱 [`docs/TECHNICAL_DEBT.md`](TECHNICAL_DEBT.md)。預估總工作量：**約 18 人/週**。

### 關鍵限制說明

1. **僅支援單一 Backend** (TD-005, P0) — 無法抽換為 Whisper.cpp、TensorRT 或其他推論引擎；為 Phase 7 架構重構的核心阻礙。
2. **Child Process 無法優雅關閉** (TD-002, P1) — audio.cpp server 沒有 SIGTERM handler，Runtime 需回退到強制終止，影響資料一致性。
3. **無即時串流** (TD-007, P1) — 雖有 `/v1/tasks/stream` 端點，但實際為同步請求，無 SSE/WebSocket 支援。
4. **無傳輸加密與認證** (TD-012, P1) — API 通訊為明文 HTTP，無 API Key 或 JWT 認證。
5. **Race Test 無法執行** (TD-003, P2) — Windows 開發環境缺少 gcc/cgo toolchain，`go test -race` 無法執行。
6. **測試覆蓋率缺口** (TD-014, P2) — CI 排除 `./tests/` 套件；邊界情況與壓力測試覆蓋不足。
7. **日誌系統原始** (TD-013, P2) — 使用 `log.Printf`，無結構化欄位、無級別控制、無輪轉。
8. **缺乏持續監控** (TD-015, P2) — 僅有一次性的 Diagnostics API，無歷史趨勢與告警。

---

## 下一步 (Next Steps)

### Phase 7 — Backend 抽象層與核心擴充

**代號**: `phase-7-backend-abstraction`  
**預估時程**: 8~10 週  
**優先級**: P0 (最高)

#### 核心目標

1. **Backend Contract Interface** (7.1, P0, TD-005) — 定義 `internal/backend` 介面，將 `internal/audiocpp` 具體型別從 Runtime 與 API 層解耦，支援多 Backend 可插拔。
2. **結構化日誌系統** (7.2, P2, TD-013) — 導入 `log/slog`，統一日誌級別與格式。
3. **HTTPS 與 API 認證** (7.3, P1, TD-012) — TLS 支援 + API Key 認證 middleware。
4. **Backend Version Endpoint** (7.4, P2, TD-001) — 在 Go Runtime 層自行實作版本查詢機制，移除 `t.Skip`。
5. **Backend Capabilities 擴充** (7.5, P2, TD-004) — 外部配置驅動的能力註冊 + 動態能力探測。
6. **Streaming Pipeline** (7.6, P1, TD-007) — SSE 為第一優先，支援 ASR/TTS 即時串流。
7. **Scheduler 與 Queue 改進** (7.7, P2) — SQLite-backed queue、延遲任務、週期性任務。
8. **輕量 Metrics** (7.8, P2, TD-015) — 基於 Diagnostics API 擴充，含歷史趨勢與請求計數。
9. **Lazy Load 衝突修復** (7.9, P2, TD-011) — 調查 `lazy_load=true` + `model_spec_override` 衝突。
10. **Race Test 與 CI 改善** (7.10, P2, TD-003+TD-014) — CI 新增 Ubuntu race test runner，修復 `./tests/` 套件。

#### 建議執行順序

```
第一波 (並行):
  ├── 7.1 Backend Interface (起點)
  ├── 7.2 結構化日誌 (低風險)
  ├── 7.7 Scheduler/Queue 改進 (獨立)
  └── 7.9 Lazy Load 修復 (獨立)
第二波 (7.1 完成後):
  ├── 7.3 HTTPS/Auth
  ├── 7.4 Version Endpoint
  ├── 7.5 Capabilities 擴充
  ├── 7.6 Streaming Pipeline
  └── 7.10 CI/Race 改善
第三波 (7.7 完成後):
  └── 7.8 輕量 Metrics
```

### Phase 8 — GPU 與多 Backend 支援

**預估時程**: 10~14 週  
**前提條件**: Phase 7 完成

核心項目：CUDA Backend (8.1) → Vulkan/Metal (8.2) → TensorRT (8.3) → Batch Inference (8.4) → Model Cache (8.5) → Hot Reload (8.6)

### Phase 9 — 叢集與高可用

**預估時程**: 12~16 週  
**前提條件**: Phase 8 完成

核心項目：Cluster Membership (9.1) → 分散式排程 (9.2) → 高可用故障轉移 (9.3) → 集中監控 (9.4) → 分散式模型管理 (9.5)

---

## 附錄：核心指標總覽

| 指標 | 數值 |
|------|------|
| Go 原始檔 | ~80+ 檔案 |
| 總程式碼行數 | ~9,225 行 (Go) |
| 測試檔案 | 11 個 |
| 含測試套件 | 7 個 |
| 測試案例 | ~50+ 案例 |
| API 端點 | 15+ 端點 |
| 依賴套件 | 10 個 (全純 Go，無 CGO) |
| Go 版本 | go1.26.4 |
| 真實 Smoke | ✅ REAL_SMOKE_PASS (66.7% word match) |
| 技術債 | 15 項 (P0×1, P1×4, P2×10) |
| 預估總工作量 | ~18 人/週 |
| 下一階段 | Phase 7 — Backend 抽象層與核心擴充 |

---

> **參考文件**:
> - [`docs/TECHNICAL_DEBT.md`](TECHNICAL_DEBT.md) — 詳細技術債清單與優先級
> - [`docs/ROADMAP.md`](ROADMAP.md) — Phase 7~9 完整路線圖
> - [`docs/releases/v0.6.md`](releases/v0.6.md) — v0.6 發布說明
> - [`docs/adr/`](adr/) — 架構決策記錄 (ADR-0001~0005)
