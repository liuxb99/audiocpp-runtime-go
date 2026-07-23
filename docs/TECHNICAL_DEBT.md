# Technical Debt 清單

> 文件版本: v1.0
> 建立日期: 2025-07-23
> 基準提交: f009937

本文件記錄 audio.cpp-runtime-go 專案中已知的技術債 (Technical Debt)，包含已知限制 (Known Limitation)、已知缺口 (Known Gap) 以及未來改進項目。

---

## 項目清單

---

### TD-001: Backend Version Endpoint 缺失

| 欄位 | 內容 |
|------|------|
| **名稱** | Backend Version Endpoint |
| **原因** | audio.cpp server 未實作 `/v1/version` 或等效的版本查詢端點，導致 Go Runtime 的 Backend Contract Test 中 `TestBackendContract_Version` 被迫使用 `t.Skip("KNOWN GAP: backend version endpoint not implemented")` 跳過。此為 audio.cpp 上游程式碼的缺口，非 Go Runtime 本身的問題。 |
| **目前影響** | Contract test 無法驗證 backend 版本號，測試報告中需獨立列出此項為 Known Gap；無法在 Runtime 層級確認 backend binary 的版本相容性；未來多 backend 架構下無法透過統一介面查詢版本。 |
| **是否阻塞** | 否 — 不影響現有 ASR/TTS 功能，但阻塞 Version Contract 測試的完整驗證。 |
| **優先級** | P2 — 一般優先級（需上游支援或自建版本查詢機制） |
| **預估工作量** | 2 人/天（若在 audio.cpp server 新增 `/v1/version` endpoint） |
| **建議 Phase** | Phase 7 — 與 Backend Contract 標準化合併處理 |

---

### TD-002: audio.cpp Child Process 缺乏優雅關閉

| 欄位 | 內容 |
|------|------|
| **名稱** | audio.cpp Child Process Graceful Shutdown 未實作 |
| **原因** | audio.cpp server 子程序未實作 SIGTERM (Windows 上為 `CTRL_BREAK_EVENT`) 處理器，導致 Go Runtime 呼叫 `StopGraceful()`（透過 `platform.StopGraceful()` 執行 `taskkill /PID` 不帶 `/F`）時，子程序不會自行退出。Runtime 在等待 5 秒後仍需回退到 `taskkill /T /F` 強制終止。在 Unix 平台上，`StopGraceful()` 發送 `SIGTERM` 同樣因 audio.cpp 缺少 signal handler 而無效。 |
| **目前影響** | Real smoke test 中 shutdown 階段無法達成完全優雅關閉（`GracefulExited=false, ForceKillUsed=true`），影響 smoke 驗收的品質指標；強制終止可能導致音訊檔案寫入不完整或資料庫狀態不一致。**Phase 7B 真實 Smoke 實測**（Source Commit a7fcde7）：Graceful stop command 返回 exit status 1；Child 超過 5 秒 graceful deadline 未退出；Runtime 內部執行 force kill 終止 child；Runtime 自身優雅退出（External Force Kill=False）；Child Process Exited=True（via internal force kill）。此為 audio.cpp 上游程式碼的已知限制，非 Go Runtime 本身可解決。 |
| **是否阻塞** | 否 — 關閉功能可正常運作，但無法保證完全優雅。 |
| **優先級** | P1 — 重要（影響 smoke 驗收品質與資料一致性） |
| **預估工作量** | 3 人/天（需修改 audio.cpp server 新增 signal handler + 跨平台測試） |
| **建議 Phase** | Phase 7C 或 Phase 8 — 需 audio.cpp 上游支援 signal handler |

---

### TD-003: Race Test 無法在 Windows 執行

| 欄位 | 內容 |
|------|------|
| **名稱** | Race Detector 在 Windows 環境無法執行 |
| **原因** | Go 的 `-race` flag 依賴於 C/C++ 執行時期 (thread sanitizer)，需要 gcc 或 cgo toolchain。Windows 開發環境多數未安裝 MinGW-w64 或 TDM-GCC，導致 `go test -race ./...` 無法執行。此限制在 Phase 6 多個子階段中已被記錄為「NOT RUN + 真實原因」。 |
| **目前影響** | 無法在 Windows 開發環境檢測 data race 問題；CI/CD 未設定 race test 步驟，可能遺漏併發 bug。 |
| **是否阻塞** | 否 — 不影響建置與功能測試，但降低併發穩定性保障。 |
| **優先級** | P2 — 一般優先級（可透過 CI 或開發環境改善解決） |
| **預估工作量** | 1 人/天（安裝 MinGW-w64 toolchain 並設定 CI 步驟） |
| **建議 Phase** | Phase 7 — 與開發工具鏈改善合併處理 |

---

### TD-004: Backend Capabilities 缺乏可插拔擴充機制

| 欄位 | 內容 |
|------|------|
| **名稱** | Backend Capabilities 擴充機制不足 |
| **原因** | `internal/audiocpp/capabilities.go` 使用靜態的 `taskToCaps` 和 `capToTask` 字典（Go map）定義能力映射。若要新增能力或支援新的 audio.cpp 上游能力，需手動修改程式碼並重新編譯。缺乏外部配置驅動或 plugin 式的擴充機制。 |
| **目前影響** | 新增能力需修改 Go 原始碼 → PR → 重新部署；第三方或社群貢獻者無法透過設定檔擴充能力；難以動態探測 audio.cpp server 的實際支援能力。 |
| **是否阻塞** | 否 — 目前能力清單已涵蓋主要功能。 |
| **優先級** | P2 — 一般優先級（影響擴充性但非功能性） |
| **預估工作量** | 3 人/天（設計動態能力探測 + YAML/JSON 配置驅動能力註冊） |
| **建議 Phase** | Phase 7 — 與 Backend 抽象層設計合併處理 |

---

### TD-005: 單一 Backend 限制（無 Backend 抽象層）

| 欄位 | 內容 |
|------|------|
| **名稱** | 僅支援 audio.cpp 單一 Backend |
| **原因** | 整個程式碼庫將 `internal/audiocpp` 的具體型別（`*audiocpp.Process`, `*audiocpp.Client`, `*audiocpp.CLIExecutor`）直接注入到 Runtime、API Server 等上層元件。沒有定義 Backend 介面（Interface），無法抽換為其他 Backend（如 Whisper.cpp、TensorRT、自研引擎）。 |
| **目前影響** | 無法支援多種推論引擎；若要新增 Backend 需大幅修改 Runtime 與 API 層；無法進行 Backend 間的 A/B 測試；限制了專案的生態擴展性。 |
| **是否阻塞** | 是 — 阻止多 Backend 支援與引擎替換的架構需求。 |
| **優先級** | P0 — 緊急（為 Phase 7 架構重構的核心阻礙） |
| **預估工作量** | 2 人/週（定義 Backend Contract Interface + 重構 Runtime + 建立 Factory） |
| **建議 Phase** | Phase 7 |

---

### TD-006: GPU Backend 未實作

| 欄位 | 內容 |
|------|------|
| **名稱** | GPU Backend 未實作（僅 CPU） |
| **原因** | 雖然 `config.go` 的配置驗證允許 `cuda`, `vulkan`, `metal` 等後端值，且 `DefaultConfig()` 預設為 `cuda`，但實際 Go Runtime 僅將此字串傳遞給 audio.cpp server 的 `--backend` 參數。真正的 GPU 支援完全依賴於 audio.cpp 是否以對應選項（`ENGINE_ENABLE_CUDA`、`ENGINE_ENABLE_VULKAN`、`ENGINE_ENABLE_METAL`）編譯。Go Runtime 層沒有 GPU 記憶體管理、裝置選擇、多 GPU 排程等能力。 |
| **目前影響** | 大型模型（如 Whisper large-v3）在 CPU 上推論速度慢（RTF > 1），無法滿足即時場景；無法利用 GPU 加速批量推理；限制了支援的模型規模。 |
| **是否阻塞** | 否 — 但嚴重影響效能與可支援的模型規模。 |
| **優先級** | P1 — 重要（影響生產部署的效能與吞吐量） |
| **預估工作量** | 2 人/週（CUDA 支援）+ 各 1 人/週（Vulkan/Metal） |
| **建議 Phase** | Phase 8 — 與多 Backend 架構合併處理 |

---

### TD-007: Streaming Pipeline 尚未實作

| 欄位 | 內容 |
|------|------|
| **名稱** | Real-time Streaming 未實作 |
| **原因** | 雖然 `TranscribeRequest` 結構體定義了 `Stream bool` 欄位且 API 端點有 `/v1/tasks/stream`，但實際 handler 實作為同步請求—等待完整結果後才回應。沒有實現 chunked transfer encoding、WebSocket、SSE (Server-Sent Events) 等即時串流通訊協定。Worker pool 也僅支援一次處理一個完整任務。 |
| **目前影響** | 無法支援即時 ASR（語音辨識過程中逐步顯示文字）；無法串流 TTS 音訊（需等待完整音訊生成才能播放）；無法支援雙向語音對話場景；限制了產品的即時體驗。 |
| **是否阻塞** | 否 — 現有同步 API 可滿足非即時場景。 |
| **優先級** | P1 — 重要（核心差異化功能） |
| **預估工作量** | 2 人/週（Streaming pipeline 設計 + SSE/WebSocket 支援 + 測試） |
| **建議 Phase** | Phase 7 |

---

### TD-008: Batch Inference 未支援

| 欄位 | 內容 |
|------|------|
| **名稱** | Batch Inference（批次推理）未實作 |
| **原因** | Job Queue 雖然支援優先級排程與 Worker Pool，但 Worker 為逐個處理任務（dequeue → process → complete），沒有批次收集與合併機制。無法將多個 ASR/TTS 請求合併為一次 batch 推理以提升 GPU 利用率。 |
| **目前影響** | 高吞吐場景下 GPU 利用率低落（若未來支援 GPU）；每個請求獨立佔用推理資源，無法共享 KV cache 或中間結果；大量小請求時 overhead 明顯。 |
| **是否阻塞** | 否 — 但影響未來高吞吐部署的效能。 |
| **優先級** | P2 — 一般優先級（需 GPU backend 就緒後才有顯著效益） |
| **預估工作量** | 1 人/週（Batch scheduler + 合併策略 + 測試） |
| **建議 Phase** | Phase 8 — 與 GPU Backend 合併處理 |

---

### TD-009: Hot Reload 未實作

| 欄位 | 內容 |
|------|------|
| **名稱** | 模型/設定 Hot Reload 未實作 |
| **原因** | Runtime 啟動時一次性讀取 YAML 設定檔並載入模型。若需更換模型或更新設定（如調整 threads、device），必須：
1. 發送 `POST /v1/shutdown`
2. 修改設定檔
3. 重新啟動 `audiocpp-runtime.exe`

沒有 `POST /v1/reload` 或類似機制的熱更新功能。 |
| **目前影響** | 模型更新或設定調整需要服務下線，導致短暫服務中斷；無法在不影響線上服務的情況下新增/移除模型；開發迭代速度受限。 |
| **是否阻塞** | 否 — 但影響運維效率與服務可用性。 |
| **優先級** | P2 — 一般優先級 |
| **預估工作量** | 2 人/週（Hot reload API + 模型動態載入/卸載 + 設定熱更新） |
| **建議 Phase** | Phase 8 |

---

### TD-010: Model Cache 未實作

| 欄位 | 內容 |
|------|------|
| **名稱** | Model Cache 機制未實作 |
| **原因** | Runtime 每次啟動時透過 audio.cpp server 重新載入模型，沒有模型快取層。模型檔案（尤其是 GGUF 格式）需從磁碟讀取並載入記憶體，大型模型（如 Whisper large-v3 > 3GB）載入時間長。沒有記憶體中的模型快取池（Model Pool）供多個 Runtime 實例共享。 |
| **目前影響** | Runtime 重啟後模型載入時間長（大型模型 > 30 秒）；多個 Runtime 實例無法共享模型記憶體，浪費 RAM；無法支援模型版本管理或模型原地更新。 |
| **是否阻塞** | 否 — 不影響功能正確性。 |
| **優先級** | P2 — 一般優先級（影響啟動時間與記憶體效率） |
| **預估工作量** | 1 人/週（設計 Model Cache + 記憶體映射 + 測試） |
| **建議 Phase** | Phase 8 |

---

### TD-011: Lazy Load 與 Model Spec Override 衝突

| 欄位 | 內容 |
|------|------|
| **名稱** | `lazy_load=true` 時 `model_spec_override` 失效 |
| **原因** | audio.cpp server 的行為：當 `lazy_load` 設為 `true` 時，模型不會在 server 啟動時預先載入，而是在首次請求時才載入。然而 `model_spec_override` 的效果依賴於模型載入流程，若模型尚未載入，override 設定可能無法正確套用。Go Runtime 的設定傳遞鏈（YAML → Config → Runtime → Process → audiocpp_server.json）雖然已接通，但未處理此衝突。 |
| **目前影響** | 使用 `lazy_load=true` 的場景無法正確套用自訂模型規格（model_spec）；使用者必須在懶載入與自訂模型規格之間二選一；此問題已在 Phase 6 的 smoke audit 記錄但仍未解決。 |
| **是否阻塞** | 否 — 可透過設 `lazy_load=false` 繞過。 |
| **優先級** | P2 — 一般優先級 |
| **預估工作量** | 2 人/天（調查 audio.cpp 的 lazy load + model_spec 互動行為 + 實作 workaround） |
| **建議 Phase** | Phase 7 |

---

### TD-012: HTTPS / 傳輸加密與 API 認證缺失

| 欄位 | 內容 |
|------|------|
| **名稱** | HTTPS 與 API 認證未實作 |
| **原因** | `internal/api/server.go` 使用標準 HTTP (`http.ListenAndServe()`) 而非 HTTPS (`http.ListenAndServeTLS()`)，且無任何 API 金鑰、JWT、或 OAuth 認證機制。`routes.go` 的 middleware 僅包含 CORS 與 shutdown 阻斷，無認證或授權檢查。 |
| **目前影響** | API 通訊為明文傳輸，敏感資料（如音訊內容、模型路徑）可被中間人竊聽；任何知道 API 位址者均可呼叫所有端點（包括 `POST /v1/shutdown`）；不符合企業部署安全規範。 |
| **是否阻塞** | 否 — 現階段主要為本機開發與內部網路使用。 |
| **優先級** | P1 — 重要（影響生產部署安全性） |
| **預估工作量** | 3 人/天（TLS 憑證管理 + API Key 認證 + middleware + 測試） |
| **建議 Phase** | Phase 7 |

---

### TD-013: 日誌系統缺乏結構化

| 欄位 | 內容 |
|------|------|
| **名稱** | 日誌系統缺乏結構化與級別控制 |
| **原因** | 整個專案使用 Go 標準 library `log.Printf` 進行日誌輸出，沒有日誌層級（DEBUG/INFO/WARN/ERROR）、沒有結構化欄位（JSON 格式）、沒有日誌輪轉（log rotation）、沒有可設定的日誌輸出目標。各元件使用 prefix（如 `[runtime]`, `[audiocpp]`, `[api]`）來區分來源，但格式不統一。 |
| **目前影響** | 生產環境除錯困難，無法快速過濾特定級別的日誌；缺乏結構化欄位無法匯入日誌分析系統（如 ELK、Loki）；長時間運行的 Runtime 日誌檔案無限制增長，可能耗盡磁碟空間；無法與 OpenTelemetry 等觀測框架整合。 |
| **是否阻塞** | 否 — 不影響功能運作。 |
| **優先級** | P2 — 一般優先級 |
| **預估工作量** | 3 人/天（導入結構化日誌 library + 級別控制 + 輪轉設定） |
| **建議 Phase** | Phase 7 |

---

### TD-014: 測試覆蓋率與 CI 缺口

| 欄位 | 內容 |
|------|------|
| **名稱** | 測試覆蓋率不足且 CI 排除上層測試 |
| **原因** | `tests/integration_test.go` 和 `tests/integration_db_test.go` 雖然存在，但 Phase 6 的 CI 工作流程（`.github/workflows/real-smoke.yml`）僅為 `workflow_dispatch` 觸發，且明確排除 `./tests/` 套件（文件記載 "Top-level /tests package still excluded from CI (pre-existing broken tests)"）。此外，單元測試主要涵蓋 happy path，邊界情況（如超大音訊、空請求、併發競態）覆蓋不足。 |
| **目前影響** | CI 無法自動執行整合測試，回歸風險高；`go test ./...` 的結果不完全反映實際品質；缺少壓力測試與穩定性測試。 |
| **是否阻塞** | 否 — 但增加回歸風險。 |
| **優先級** | P2 — 一般優先級 |
| **預估工作量** | 1 人/週（修復整合測試 + 新增邊界測試 + 設定 CI 自動化） |
| **建議 Phase** | Phase 7 |

---

### TD-015: Lack of Metrics / Monitoring（缺乏持續監控）

| 欄位 | 內容 |
|------|------|
| **名稱** | Runtime 缺乏持續監控與 Metrics 匯出 |
| **原因** | `internal/runtime/diagnostics.go` 提供了執行緒快照（`Diagnostics` 結構體），可透過 `GET /v1/runtime/diagnostics` 獲取一次性狀態。但沒有持續的 metrics 收集機制、沒有 Prometheus 或其他標準 metrics 格式的匯出端點、沒有閾值告警能力。根據 Phase 6D 的 ADR-0005 決策，刻意保持輕量不引入 Prometheus。 |
| **目前影響** | 無法即時監控 Runtime 健康狀態趨勢（如記憶體增長、佇列堆積、請求延遲分佈）；生產環境故障排查需依賴手動呼叫 diagnostics API；缺乏 SLO/SLI 的度量基礎。 |
| **是否阻塞** | 否 — 不影響功能，但影響運維可觀測性。 |
| **優先級** | P2 — 一般優先級 |
| **預估工作量** | 1 人/週（設計輕量 Metrics API + 歷史趨勢儲存 + 基本告警） |
| **建議 Phase** | Phase 7 |

---

## 優先級分佈摘要

| 優先級 | 項目數 | 項目 |
|--------|--------|------|
| **P0（緊急）** | 1 | TD-005: 單一 Backend 限制 |
| **P1（重要）** | 4 | TD-002 (Graceful Shutdown), TD-006 (GPU), TD-007 (Streaming), TD-012 (HTTPS/Auth) |
| **P2（一般）** | 10 | TD-001 (Version), TD-003 (Race), TD-004 (Capabilities), TD-008 (Batch), TD-009 (Hot Reload), TD-010 (Model Cache), TD-011 (Lazy Load), TD-013 (Logging), TD-014 (Test Coverage), TD-015 (Metrics) |
| **P3（低優先）** | 0 | — |

## 工作量總計

| 類別 | 預估總工作量 |
|------|-------------|
| P0 | 2 人/週 |
| P1 | 約 9 人/週（含 GPU 估算） |
| P2 | 約 7 人/週 |
| **總計** | **約 18 人/週** |

## 建議處理順序

```
Phase 7（當前 + 下一階段）:
  ├── TD-005: Backend 抽象層（P0, 2 人/週）
  ├── TD-002: Graceful Shutdown（P1, 3 人/天）
  ├── TD-007: Streaming Pipeline（P1, 2 人/週）
  ├── TD-012: HTTPS / Auth（P1, 3 人/天）
  ├── TD-003: Race Test（P2, 1 人/天）
  ├── TD-004: Capabilities 擴充（P2, 3 人/天）
  ├── TD-011: Lazy Load 衝突（P2, 2 人/天）
  ├── TD-013: 結構化日誌（P2, 3 人/天）
  ├── TD-014: 測試覆蓋率（P2, 1 人/週）
  ├── TD-015: Metrics（P2, 1 人/週）
  └── TD-001: Version Endpoint（P2, 2 人/天）

Phase 8（下一階段）:
  ├── TD-006: GPU Backend（P1, 2 人/週）
  ├── TD-008: Batch Inference（P2, 1 人/週）
  ├── TD-009: Hot Reload（P2, 2 人/週）
  └── TD-010: Model Cache（P2, 1 人/週）
```

---

> **說明**: 本文件基於 commit `f009937` 的程式碼分析。技術債的優先級與工作量預估會隨專案發展而調整，建議每 Phase 開始前重新評估。
