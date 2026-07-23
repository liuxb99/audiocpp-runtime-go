# 專案路線圖 — audio.cpp-runtime-go

> 文件版本: v1.0
> 建立日期: 2025-07-23
> 基準版本: v0.6 (Phase 1~6 已完成)
> 基準提交: f009937

---

## 概述

本路線圖規劃 Phase 7 ~ Phase 9 的發展方向，基於專案當前架構與技術債清單 (`docs/TECHNICAL_DEBT.md`) 擬定。每個 Phase 的範圍與優先級會隨專案演進調整，建議每季重新評估。

### 當前專案狀態摘要

| 面向 | 狀態 |
|------|------|
| Go 版本 | go1.26.4 windows/amd64 |
| 建置與測試 | go vet / build / test 全部通過 |
| 真實 Smoke | REAL_SMOKE_PASS ✅ (Citrinet 模型) |
| 技術債 | 15 項 (P0×1, P1×4, P2×10) |
| Backend 支援 | 僅 audio.cpp (CPU / CUDA 傳遞參數) |
| GPU 管理 | 無 (僅傳遞 `--backend` 參數) |
| Streaming | 未實作 (同步 API 僅) |
| 監控 | 一次性 Diagnostics API，無持續 Metrics |
| CI | 僅手動 workflow (workflow_dispatch) |

---

## Phase 7 — Backend 抽象層與核心擴充

**代號**: `phase-7-backend-abstraction`  
**預估時程**: 8~10 週 (約 2.5 個月)  
**優先級**: P0 (最高) — 為後續所有 Phase 奠定架構基礎

### 目標

1. **打破單一 Backend 限制**：建立 Backend Contract Interface，將 `internal/audiocpp` 的具體型別從 Runtime 與 API 層解耦
2. **補齊 P0/P1 技術債**：解決阻塞性與高影響力的技術債項目
3. **Streaming Pipeline 基礎**：為即時 ASR/TTS 建立傳輸層通道
4. **輕量可觀測性**：在不引入 Prometheus 的前提下提供持續 Metrics
5. **結構化日誌與安全基礎**：改善運維體驗與生產部署安全性

### 包含項目

#### 7.1 — Backend Contract Interface (P0, TD-005)

**工作量預估**: 2 人/週

- 定義 `internal/backend` 套件，包含核心 Interface：
  - `Backend` 介面：`Start()`, `Ready()`, `Health()`, `Stop()`, `ForceStop()`, `PID()`, `Version()`, `Capabilities()`
  - `BackendFactory`：根據配置字串 (`audio.cpp`, `whisper.cpp`, …) 建立對應 Backend 實例
  - `BackendOptions`：統一啟動參數結構體（Host, Port, Backend Type, Device, Threads, Models, …）
- 將 `internal/audiocpp` 改為實作 `Backend` 介面的第一個 Adapter：
  - `internal/audiocpp/adapter.go` — 將現有 `Process` + `Client` + `CLIExecutor` 包裝為 `backend.Backend`
  - 既有 `internal/audiocpp/*` 內部結構不變，僅新增適配層
- 重構 `internal/runtime/runtime.go`：
  - 將 `proc *audiocpp.Process`, `cli *audiocpp.CLIExecutor`, `client *audiocpp.Client` 替換為 `backend.Backend`
  - Runtime 不再直接依賴 `internal/audiocpp` 套件
- 重構 `internal/api/server.go`：
  - `Server` 結構體中 `audiocppCli *audiocpp.Client` 改為 `backend.Backend`
- 更新 Backend Contract Test (`internal/runtime/contract_test.go`)：
  - 從測試 `*audiocpp.Process` 改為測試 `backend.Backend`
  - 保留所有既有 contract 驗證：Start/Ready/Health/Stop/ForceStop/PID/Capabilities/Version (Known Gap)

**相依性**: 無 — 此為 Phase 7 第一個子項目
**風險**: 
- 重構範圍大，需注意現有測試的回歸
- `internal/api/server.go` 直接呼叫 `audiocppCli.Speech()`, `audiocppCli.TranscribeJSON()` 等方法，需確保 `Backend` 介面包涵全部必要方法
- 建議分兩步：先定義 Interface + Adapter，全部測試通過後再重構 Runtime/API 的依賴注入

#### 7.2 — 結構化日誌系統 (P2, TD-013)

**工作量預估**: 3 人/天

- 引入輕量結構化日誌函式庫（如 `log/slog`，Go 1.21+ 標準庫）
- 定義日誌層級：DEBUG / INFO / WARN / ERROR
- 統一日誌格式：`time level source message key=value`（JSON 輸出可選）
- 各元件統一使用新日誌系統，移除 `log.Printf` 與 `[runtime]`, `[audiocpp]`, `[api]` 等自訂 prefix
- 日誌輪轉與檔案輸出設定（可選，依賴 `internal/config` 擴充）

**相依性**: 無，可與 7.1 並行
**風險**: 低 — 標準庫 `log/slog` 無外部依賴；需注意向後相容

#### 7.3 — HTTPS 與 API 認證 (P1, TD-012)

**工作量預估**: 3 人/天

- TLS 支援：
  - `internal/config` 新增 `server.tls` 區段：`enabled`, `cert_file`, `key_file`
  - `internal/api/server.go` 依據設定選擇 `ListenAndServe()` 或 `ListenAndServeTLS()`
- API Key 認證：
  - `internal/config` 新增 `server.api_key` (可選)
  - 新增認證 middleware：檢查 `Authorization: Bearer <api_key>` header
  - 設定 API Key 時自動啟用，未設定時保持目前無認證行為（向後相容）
- `POST /v1/shutdown` 端點即便無 API Key 也應限制（或一律要求 API Key）

**相依性**: 7.1 (API Server 重構後較易插入 middleware)
**風險**: 低 — 標準庫 `crypto/tls` 無外部依賴

#### 7.4 — Backend Version Endpoint 解決方案 (P2, TD-001)

**工作量預估**: 2 人/天

- 方案 A（優先）: 在 Go Runtime 層自行實作版本查詢機制：
  - Runtime 啟動時記錄 audio.cpp binary 的檔案版本 (Windows `GetFileVersionInfo` / Unix `--version`)
  - 將版本資訊納入 Diagnostics 與 Backend Contract 的 `Version()` 方法
- 方案 B（若上游配合）: 向 audio.cpp 提交 PR 新增 `/v1/version` endpoint
- 更新 Contract Test：移除 `t.Skip`，改用真實版本驗證

**相依性**: 7.1 (Backend Interface 需含 `Version()`)
**風險**: 低 — 方案 A 不依賴上游

#### 7.5 — Backend Capabilities 擴充機制 (P2, TD-004)

**工作量預估**: 3 人/天

- 將靜態 Go map 改為外部配置驅動：
  - 定義 YAML/JSON 格式的能力註冊檔（如 `capabilities.yaml`）
  - `internal/audiocpp/capabilities.go` 支援從檔案載入能力映射
  - 保留靜態映射作為預設值（向後相容）
- 動態能力探測：
  - 透過 audio.cpp `GET /v1/models` 或 `/health` 回應自動偵測實際支援能力

**相依性**: 7.1 (Backend Interface 完成後可將 Capabilities 納入)
**風險**: 低

#### 7.6 — Streaming Pipeline (P1, TD-007)

**工作量預估**: 2 人/週

- 傳輸層選擇：**SSE (Server-Sent Events)** 為第一優先，原因：
  - 實作簡單，只需標準 HTTP
  - 與現有 OpenAI-compatible API 風格一致
  - 瀏覽器原生支援 `EventSource`
  - WebSocket 保留為 Phase 7 後的延伸選項
- ASR Streaming：
  - `internal/api/handlers_asr.go` 新增 `handleASRStream`：接收完整音訊後分段送回轉錄結果
  - 或：支援 chunked transfer 上傳（需 audio.cpp 上游支援）
- TTS Streaming：
  - `internal/api/handlers_tts.go` 新增 `handleTTSStream`：同步生成過程中以 SSE 分段送回音訊 chunk
- `internal/jobs/worker.go`：
  - 新增 Streaming Worker 型態，支援逐 chunk 回呼
- API 端點：
  - `POST /v1/audio/transcriptions?stream=true`
  - `POST /v1/audio/speech?stream=true`
  - 保留現有 `POST /v1/tasks/stream` 端點（改為實際串流行為）

**相依性**: 7.1 (API Server 重構後較易新增 handler)
**風險**: 中 — audio.cpp 上游可能不支援 chunked 輸出；需確認 streaming 模式下 audio.cpp 的行為

#### 7.7 — Scheduler 與 Queue 改進 (P2)

**工作量預估**: 1 人/週

- Queue 持久化：
  - 將記憶體 Queue (`internal/jobs/queue.go`) 改為可選的 SQLite-backed queue
  - Runtime 重啟時自動恢復未完成的任務
- Scheduler 增強：
  - 延遲任務支援 (Delayed Job)：指定 `schedule_at` 時間才 dequeue
  - 週期性任務支援 (Cron-like)：透過 `internal/jobs/model.go` 新增 `RepeatInterval` 欄位
- Queue Metrics 整合（銜接 7.8）：
  - 匯出 Queue 深度、處理速率、平均等待時間

**相依性**: 無，可與 7.1 並行
**風險**: 低 — 改動範圍侷限於 `internal/jobs/` 套件

#### 7.8 — 輕量 Metrics (P2, TD-015)

**工作量預估**: 1 人/週

**原則**: 依循 ADR-0005 決策，不引入 Prometheus，保持輕量

- 在現有 Diagnostics API (`GET /v1/runtime/diagnostics`) 基礎上擴充：
  - 加入歷史趨勢：保留最近 N 筆 diagnostics 快照（可設定量，預設 100）
  - 加入請求計數：成功/失敗次數、平均延遲、P50/P95/P99 延遲
  - 加入 Queue 指標：累計處理任務數、平均等待時間、佇列峰值長度
- 新增 `GET /v1/runtime/metrics` 端點：
  - 回傳累計指標（自 Runtime 啟動以來）
  - 回傳當前即時數值
  - JSON 格式，可被外部監控系統輪詢
- Metrics 儲存：
  - 可選 SQLite 持久化（跨重啟保留歷史趨勢，最大保留天數可設定）
  - 或僅記憶體保留（Runtime 重啟後重置）
- 不導入 Prometheus client library，不提供 `/metrics` (Prometheus 格式) 端點

**相依性**: 7.7 (Queue Metrics 需 Queue 改進完成)
**風險**: 低 — 設計保持輕量，不引入外部依賴

#### 7.9 — Lazy Load 與 Model Spec Override 衝突修復 (P2, TD-011)

**工作量預估**: 2 人/天

- 調查 audio.cpp 上游在 `lazy_load=true` 時 `model_spec_override` 的真實行為
- 若為 audio.cpp bug，提交修復 PR
- 若為設計限制，在 Go Runtime 層實作 workaround：
  - 方案：強制在首次請求前觸發一次模型載入（發送空請求），載入完成後再套用 override
  - 或：在 `lazy_load=true` 時禁止 `model_spec_override` 並回饋明確錯誤訊息

**相依性**: 無
**風險**: 低 — 有明確繞過方案 (`lazy_load=false`)

#### 7.10 — Race Test 與 CI 改善 (P2, TD-003 + TD-014)

**工作量預估**: 1 人/週

- **TD-003**: 
  - 在 CI (.github/workflows/) 新增 Ubuntu runner 執行 `go test -race ./...`
  - Windows 開發環境文件化安裝 MinGW-w64 的步驟
- **TD-014**:
  - 修復 `tests/integration_test.go` 和 `tests/integration_db_test.go` 的既有問題
  - 將 `./tests/` 套件納入 CI
  - 新增邊界測試：超大音訊檔、空請求、併發競態、Backend 崩潰恢復
  - 新增壓力測試腳本 (`scripts/stress_test.ps1`)
  - 將 CI workflow 從 `workflow_dispatch` 改為 `push` + `pull_request` 自動觸發（選擇性，視 repo 設定）

**相依性**: 7.1 (整合測試需 Backend Interface 就緒)
**風險**: 中 — CI 設定需有 GitHub Actions runner 權限；`tests/` 套件的既有問題可能需較多時間排查

### 相依性圖 (Phase 7)

```
7.1 (Backend Interface) ──┬── 7.4 (Version)
                          ├── 7.5 (Capabilities)
                          ├── 7.3 (HTTPS/Auth)
                          └── 7.6 (Streaming)
7.2 (Structured Logging)  ── 獨立，可任意並行
7.7 (Scheduler/Queue)     ──┬── 7.8 (Metrics)
7.9 (Lazy Load Fix)       ── 獨立，可任意並行
7.10 (CI/Race)            ── 部分依賴 7.1 完成後修復 tests/
```

### 風險與緩解 (Phase 7)

| 風險 | 機率 | 影響 | 緩解措施 |
|------|------|------|----------|
| Backend Interface 設計過於抽象導致實作困難 | 中 | 高 | 以 audio.cpp 為唯一實作對象設計 Interface，避免過度工程；保留向後相容 |
| audio.cpp 上游不支援 streaming 所需介面 | 中 | 中 | SSE 方案可在 Go 層自行聚合 chunk，不完全依賴上游 |
| 結構化日誌影響現有工具鏈 | 低 | 低 | 保留 `log/slog` 的 TextHandler 作為預設，JSON 可選 |
| Backend Interface 重構導致現有測試大規模失敗 | 中 | 高 | 逐步重構：先加 Interface → Adapter → 測試通過 → 再替換 Runtime/API 依賴 |
| CI 設定因權限問題無法完成 | 低 | 中 | 文件化手動測試流程作為備案 |

---

## Phase 8 — GPU 與多 Backend 支援

**代號**: `phase-8-gpu-multi-backend`  
**預估時程**: 10~14 週 (約 3~3.5 個月)  
**優先級**: P1 — 影響生產部署效能與可支援模型規模

### 目標

1. **GPU Backend 支援**：實現 CUDA Backend，充分發揮 GPU 加速能力
2. **多模型並行**：支援同一 Runtime 內多模型同時載入與推理
3. **Model Cache**：減少模型重複載入時間與記憶體浪費
4. **Hot Reload**：不中斷服務的前提下更新模型與設定
5. **Batch Inference**：提升 GPU 利用率與高吞吐場景效能

### 包含項目

#### 8.1 — CUDA Backend 實作 (P1, TD-006)

**工作量預估**: 2 人/週

- 建立新的 Backend Adapter：`internal/backend/cuda/cuda.go`
  - 實作 `backend.Backend` 介面
  - 封裝 audio.cpp 以 CUDA 編譯 (`ENGINE_ENABLE_CUDA`) 的啟動與通訊
- GPU 記憶體管理：
  - 透過 audio.cpp API 查詢 GPU 記憶體使用量
  - 將 GPU 記憶體資訊納入 Diagnostics
- 多 GPU 支援：
  - `internal/config` 新增 `device_ids` 支援多 GPU 選擇
  - 啟動時可指定使用哪個 GPU 裝置
- 效能基準測試：
  - 建立 `scripts/benchmark_gpu.ps1` 對比 CPU vs CUDA 的 RTF (Real-Time Factor)
  - 涵蓋不同模型大小（small / medium / large）

**相依性**: Phase 7.1 (Backend Interface)
**風險**: 中 — 需 audio.cpp 以 `ENGINE_ENABLE_CUDA` 編譯；CUDA 工具鏈依賴 NVIDIA 生態系

#### 8.2 — Vulkan / Metal Backend (P1, TD-006 延伸)

**工作量預估**: 各 1 人/週

- Vulkan Backend：`internal/backend/vulkan/vulkan.go`
  - 封裝 audio.cpp 以 Vulkan 編譯 (`ENGINE_ENABLE_VULKAN`) 的啟動
  - 主要適用於 Linux / Windows 非 NVIDIA GPU
- Metal Backend：`internal/backend/metal/metal.go`
  - 封裝 audio.cpp 以 Metal 編譯 (`ENGINE_ENABLE_METAL`) 的啟動
  - 僅適用於 macOS
- 各平台自動偵測：
  - Runtime 啟動時自動檢查可用 Backend
  - 若設定 `backend: auto` 則自動選擇最佳可用 Backend

**相依性**: 8.1 (CUDA Backend 的架構模式可複用)
**風險**: 高 — 需 audio.cpp 以對應選項編譯；Vulkan/Metal 的穩定性依賴上游支援程度

#### 8.3 — TensorRT 整合 (P1 延伸)

**工作量預估**: 2 人/週

- TensorRT Backend：`internal/backend/tensorrt/tensorrt.go`
  - 封裝 audio.cpp 以 TensorRT 編譯 (`ENGINE_ENABLE_TENSORRT`) 的啟動
- TensorRT 模型最佳化：
  - 支援 TensorRT engine 快取（避免每次重新最佳化）
  - 支援 FP16 / INT8 量化模式
- 效能基準測試：
  - 對比 CUDA vs TensorRT 的 RTF 與記憶體用量

**相依性**: 8.1 (CUDA 工具鏈為 TensorRT 的前置)
**風險**: 高 — TensorRT 依賴特定 GPU 架構與 driver 版本；audio.cpp 的 TensorRT 支援可能不完整

#### 8.4 — Batch Inference (P2, TD-008)

**工作量預估**: 1 人/週

- Batch Scheduler：
  - 在 `internal/jobs/` 新增 `BatchScheduler`
  - 收集相同 task type 的多個請求，等待 batch_window 時間或收集到 batch_size 個後一次提交
  - 設定項目：`batch_size`, `batch_window_ms` (預設 0 = 關閉 batch)
- Worker 改造：
  - `internal/jobs/worker.go` 支援 batch worker 模式
  - 將 batch 請求合併為 audio.cpp 的 batch API 呼叫
- 回退策略：
  - 若 audio.cpp 不支援 batch API，自動回退到逐個處理

**相依性**: 8.1 (Batch 在 GPU 上效益最顯著)
**風險**: 中 — audio.cpp 上游 batch API 支援程度未知

#### 8.5 — Model Cache (P2, TD-010)

**工作量預估**: 1 人/週

- 記憶體模型快取池：
  - `internal/models/cache.go` — 以 `sync.Map` 或 LRU Cache 實作
  - 支援多個 Runtime 實例共享 GGUF 模型的記憶體映射 (`mmap`)
- 快取策略：
  - LRU (Least Recently Used) 淘汰
  - 可設定最大快取大小 (`cache_size_gb`)
  - 熱模型常駐，冷模型自動卸載
- 整合至 Runtime：
  - Runtime 啟動時先檢查 Cache，避免重複載入
  - 模型切換時優先從 Cache 讀取

**相依性**: 8.1 (GPU 記憶體管理與 Model Cache 需協調)
**風險**: 低 — 可先在 CPU 模式實作，GPU 模式疊加

#### 8.6 — Hot Reload (P2, TD-009)

**工作量預估**: 2 人/週

- API 端點：
  - `POST /v1/reload` — 熱重新載入設定檔與模型
  - `POST /v1/models/{id}/reload` — 重新載入特定模型
  - `DELETE /v1/models/{id}` — 卸載特定模型（不影響其他模型）
- 設定熱更新：
  - Runtime 監聽設定檔變更（透過 `fsnotify` 或輪詢）
  - 偵測到變更後自動套用非破壞性設定（不影響正在處理的請求）
- 模型動態載入/卸載：
  - 透過 audio.cpp API 新增/移除模型
  - 正在使用目標模型的請求完成後才卸載（drain 策略）
- 狀態管理：
  - Hot Reload 期間 Runtime 狀態不變（維持 Running）
  - 若 Hot Reload 失敗，回滾至前一組設定

**相依性**: 8.5 (Model Cache 提供 Hot Reload 所需的記憶體管理基礎)
**風險**: 中 — 需 audio.cpp 支援動態模型管理；設定檔監聽引入外部依賴 (`fsnotify`)

### 相依性圖 (Phase 8)

```
8.1 (CUDA Backend) ──┬── 8.2 (Vulkan/Metal)
                     ├── 8.3 (TensorRT)
                     ├── 8.4 (Batch Inference)
                     └── 8.5 (Model Cache) ── 8.6 (Hot Reload)
```

### 風險與緩解 (Phase 8)

| 風險 | 機率 | 影響 | 緩解措施 |
|------|------|------|----------|
| audio.cpp 上游 GPU 支援不穩定 | 中 | 高 | 在 Backend Adapter 層封裝差異，GPU 降級時優雅回退至 CPU |
| CUDA 工具鏈版本相容性問題 | 低 | 中 | 鎖定 CUDA 11.8 / 12.x 並文件化 |
| TensorRT 整合難度高 | 高 | 中 | 列為 Optional Backend，不影響其他功能交付 |
| Vulkan/Metal 上游維護度低 | 中 | 低 | 標示為「社羣貢獻優先」而非承諾支援 |
| Hot Reload 導致狀態不一致 | 低 | 高 | 完整的 rollback 機制 + 自動化測試 |

---

## Phase 9 — 叢集與高可用

**代號**: `phase-9-cluster-ha`  
**預估時程**: 12~16 週 (約 3~4 個月)  
**優先級**: P2 — 在單機效能最佳化完成後啟動

### 目標

1. **分散式 Runtime**：支援多機部署與協作
2. **負載平衡**：在多個 Runtime 實例間智慧分配請求
3. **高可用架構**：無單點故障，自動故障轉移
4. **集中式監控**：統一管理所有節點的狀態與指標

### 包含項目

#### 9.1 — Cluster Membership 與服務發現

**工作量預估**: 2 人/週

- 節點註冊與心跳：
  - 新增 `internal/cluster/` 套件
  - 節點啟動時向 Cluster Registry 註冊
  - 定期發送心跳 (heartbeat) 更新節點狀態
- 服務發現機制：
  - 方案 A（輕量）: 基於 SQLite 共享資料庫的節點表
  - 方案 B（標準）: 整合 etcd / Consul 作為 Cluster Store
  - 優先實作方案 A，視需求升級至方案 B
- 節點狀態：
  - `alive` / `draining` / `dead`
  - 節點離開時自動標記 `dead` (基於心跳逾時)

**相依性**: Phase 8.1 (單機 GPU 最佳化完成後才有多節點需求)
**風險**: 中 — 服務發現機制選擇會影響整體架構複雜度

#### 9.2 — 分散式排程與負載平衡

**工作量預估**: 2 人/週

- 全域任務佇列：
  - 將 `internal/jobs/queue.go` 從記憶體佇列改造為分散式佇列
  - 後端支援：SQLite (方案 A) / Redis / RabbitMQ (方案 B)
- 負載平衡策略：
  - Round-Robin（預設）
  - Least-Connections（最少活躍請求優先）
  - Resource-Based（根據節點 CPU/GPU 使用率）
- 請求路由：
  - 支援模型親和性 (Model Affinity)：相同模型的請求路由到已載入該模型的節點
  - 支援地域親和性 (Zone Affinity)：就近路由降低延遲

**相依性**: 9.1 (需節點註冊機制)
**風險**: 中 — 分散式佇列選擇影響部署複雜度

#### 9.3 — 高可用與故障轉移

**工作量預估**: 2 人/週

- 節點故障偵測：
  - 基於心跳逾時 (可設定：預設 15 秒)
  - 主動健康檢查：定期向節點 `/health` 發送請求
- 自動故障轉移：
  - 故障節點上的任務自動重新排隊 (requeue)
  - 重新排隊策略：延遲重試 (exponential backoff)，最大重試次數
  - 轉移過程中正在處理的任務設置為 `orphaned` 狀態
- Leader 選舉：
  - 輕量 Leader 選舉機制 (基於 SQLite 或 etcd)
  - Leader 負責排程決策與 Cluster Registry 維護
  - Leader 故障時自動重新選舉

**相依性**: 9.1, 9.2
**風險**: 高 — Leader 選舉與分散式共識是複雜問題；建議使用 etcd + 既有的選舉 library

#### 9.4 — 集中式監控與管理

**工作量預估**: 2 人/週

- 集中式 Metrics 聚合：
  - 各節點定期將 Metrics 推送至 Cluster Registry (或由 Registry 輪詢)
  - 聚合指標：叢集層級的請求量、延遲、錯誤率、GPU 利用率
  - 保留 Phase 7.8 的「不引入 Prometheus」原則；改為自建聚合 API
- 管理 Dashboard API：
  - `GET /v1/cluster/nodes` — 列出所有節點與狀態
  - `GET /v1/cluster/stats` — 叢集統計資訊
  - `GET /v1/cluster/nodes/{id}` — 特定節點詳細資訊
  - `POST /v1/cluster/nodes/{id}/drain` — 優雅關閉節點（排空後轉移）
- 告警基礎：
  - 基於閾值的簡單告警規則（可設定 YAML 配置）
  - 告警渠道：Webhook / Log
  - 不引入獨立告警系統（如 AlertManager），保持輕量

**相依性**: 9.1 (需節點概念), Phase 7.8 (Metrics 基礎)
**風險**: 中 — 自建監控系統 vs 整合成熟方案 (Prometheus + Grafana) 的抉擇；ADR 建議維持輕量，但若 Phase 9 時團隊規模擴大，應重新評估

#### 9.5 — 分散式模型管理

**工作量預估**: 1 人/週

- 模型同步：
  - 新節點加入時自動拉取模型清單與設定
  - 支援模型檔案分散式儲存（NFS / S3 / MinIO）
- 模型版本管理：
  - 模型更新時不影響正在使用舊版本的節點
  - 支援灰度發佈：先更新部分節點，驗證後再全面更新
- 模型分發策略：
  - 所有節點載入所有模型（高記憶體消耗）
  - 按需載入：僅當請求路由到該節點時才載入模型（延遲較高，但節省記憶體）

**相依性**: 8.5 (Model Cache), 8.6 (Hot Reload)
**風險**: 中 — 模型檔案同步與版本管理在分散式環境中較複雜

### 相依性圖 (Phase 9)

```
9.1 (Cluster Membership) ──┬── 9.2 (Distributed Scheduling)
                          ├── 9.4 (Centralized Monitoring)
                          └── 9.5 (Distributed Model Mgmt)
9.2 (Distributed Scheduling) ── 9.3 (HA & Failover)
9.4 (Centralized Monitoring) ── 需要 Phase 7.8 Metrics 基礎
9.5 (Distributed Model Mgmt) ── 需要 Phase 8.5 + 8.6
```

### 風險與緩解 (Phase 9)

| 風險 | 機率 | 影響 | 緩解措施 |
|------|------|------|----------|
| 分散式架構設計不當導致資料不一致 | 中 | 高 | 以 etcd 作為強一致後端；Critical 路徑加入事務保證 |
| Leader 選舉實作複雜度高 | 高 | 中 | 使用 etcd + `concurrency` 套件或 `go.etcd.io/etcd/client/v3/concurrency` |
| 節點間網路延遲影響排程效能 | 中 | 中 | 支援多種排程策略，Region/Zone 感知 |
| 叢集規模擴大後 SQLite 成為瓶頸 | 中 | 高 | 提供可插拔後端（SQLite → PostgreSQL / etcd） |
| 專案團隊規模不足以支援分散式開發 | 高 | 高 | 考慮將 Phase 9 拆分為多個子 Phase，優先實作最小可用叢集 |

---

## 總時程總覽

| Phase | 主要內容 | 預估工期 | 技術債處理 | 開始條件 |
|-------|---------|---------|-----------|---------|
| **Phase 7** | Backend Interface + Streaming + Metrics + Logging + HTTPS + CI | 8~10 週 | TD-002, TD-005, TD-007, TD-012 (P0/P1) + TD-001, TD-003, TD-004, TD-011, TD-013, TD-014, TD-015 (P2) | Phase 6 封板完成 |
| **Phase 8** | CUDA/Vulkan/Metal Backend + TensorRT + Batch + Model Cache + Hot Reload | 10~14 週 | TD-006, TD-008, TD-009, TD-010 | Phase 7 完成 |
| **Phase 9** | 叢集 + 負載平衡 + HA + 集中監控 + 分散式模型管理 | 12~16 週 | — (新功能為主) | Phase 8 完成 |

### 建議 Phase 內子項目優先順序

```
Phase 7:
  └── 第一波 (並行啟動):
       ├── 7.1 Backend Interface (起點，不可缺)
       ├── 7.2 結構化日誌 (低風險，立即改善)
       ├── 7.7 Scheduler/Queue 改進 (獨立)
       └── 7.9 Lazy Load 修復 (獨立)
  └── 第二波 (7.1 完成後):
       ├── 7.3 HTTPS/Auth
       ├── 7.4 Version Endpoint
       ├── 7.5 Capabilities 擴充
       ├── 7.6 Streaming Pipeline
       └── 7.10 CI/Race 改善
  └── 第三波 (7.7 完成後):
       └── 7.8 輕量 Metrics

Phase 8:
  └── 第一波: 8.1 CUDA Backend (核心)
  └── 第二波: 8.4 Batch + 8.5 Model Cache (與第一波部分並行)
  └── 第三波: 8.2 Vulkan/Metal + 8.3 TensorRT (可選)
  └── 第四波: 8.6 Hot Reload (需 Model Cache)

Phase 9:
  └── 第一波: 9.1 Cluster Membership + 9.4 集中監控 (並行啟動)
  └── 第二波: 9.2 分散式排程 + 9.5 模型管理 (並行)
  └── 第三波: 9.3 高可用與故障轉移 (需 9.1+9.2)
```

---

## 決定點 (Decision Gates)

每個 Phase 結束時設定檢查點，決定是否進入下一 Phase：

| 檢查點 | 條件 | 若未通過 |
|--------|------|----------|
| **Gate 7→8** | Phase 7 全部子項目完成測試 ≥90% 涵蓋率；Backend Interface Contract Test 全部 PASS (Version 除外)；Streaming 可運作 | 延長 Phase 7 或縮小 Phase 8 範圍 |
| **Gate 8→9** | GPU Backend Benchmark RTF < 1.0；Model Cache 命中率 ≥80%；Hot Reload 不中斷服務 | 延後 Phase 9，優先強化 Phase 8 |
| **Gate 9→v1.0** | 叢集 ≥3 節點可運作；故障轉移 < 30 秒；集中監控儀表板可用 | 不宣告 v1.0，繼續迭代 |

---

## 附錄：技術債對應表

| 技術債編號 | 名稱 | 優先級 | 處理 Phase | 工作量 |
|-----------|------|--------|-----------|--------|
| TD-005 | 單一 Backend 限制 | P0 | **Phase 7** | 2 人/週 |
| TD-002 | Graceful Shutdown | P1 | **Phase 7** (部分已在 Phase 6 改善，持續優化) | 3 人/天 |
| TD-007 | Streaming Pipeline | P1 | **Phase 7** | 2 人/週 |
| TD-012 | HTTPS / Auth | P1 | **Phase 7** | 3 人/天 |
| TD-001 | Version Endpoint | P2 | **Phase 7** | 2 人/天 |
| TD-003 | Race Test | P2 | **Phase 7** | 1 人/天 |
| TD-004 | Capabilities 擴充 | P2 | **Phase 7** | 3 人/天 |
| TD-011 | Lazy Load 衝突 | P2 | **Phase 7** | 2 人/天 |
| TD-013 | 結構化日誌 | P2 | **Phase 7** | 3 人/天 |
| TD-014 | 測試覆蓋率 | P2 | **Phase 7** | 1 人/週 |
| TD-015 | Metrics | P2 | **Phase 7** | 1 人/週 |
| TD-006 | GPU Backend | P1 | **Phase 8** | 2+ 人/週 |
| TD-008 | Batch Inference | P2 | **Phase 8** | 1 人/週 |
| TD-009 | Hot Reload | P2 | **Phase 8** | 2 人/週 |
| TD-010 | Model Cache | P2 | **Phase 8** | 1 人/週 |

---

> **文件維護**：本路線圖應在每個 Phase 開始前重新評估與更新。實際進度與預估有偏差時，優先調整範圍而非截止日。
