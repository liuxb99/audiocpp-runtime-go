# Architecture

## Overview

```
┌─────────────────────────────────────────────────┐
│                 audiocpp-runtime.exe            │
│                                                   │
│  ┌──────────┐   ┌──────────┐   ┌──────────────┐ │
│  │ API      │   │ Job      │   │ Model        │ │
│  │ Server   │◄─►│ Manager  │◄─►│ Registry     │ │
│  └────┬─────┘   └────┬─────┘   └──────┬───────┘ │
│       │              │                │          │
│  ┌────▼─────┐   ┌────▼─────┐   ┌──────▼───────┐ │
│  │ AudioC++ │   │ Worker   │   │ Output       │ │
│  │ Client   │   │ Pool     │   │ Manager      │ │
│  └────┬─────┘   └──────────┘   └──────────────┘ │
│       │                                           │
│  ┌────▼─────┐                                     │
│  │ AudioC++ │   ┌────────────┐   ┌────────────┐  │
│  │ Process  │   │ SQLite DB  │   │ Model      │  │
│  │ Manager  │   │ (Storage)  │   │ Manifests  │  │
│  └──────────┘   └────────────┘   └────────────┘  │
└─────────────────────────────────────────────────┘
         │ HTTP API
         ▼
┌──────────────────┐
│ audiocpp_server  │  (subprocess)
│ ─ C++ inference  │
│ ─ Model loading   │
│ ─ Audio processing│
└──────────────────┘
```

## Layers

### 1. API Layer (`internal/api/`)
HTTP server layer using gorilla/mux. Handles request routing, authentication (planned), and response formatting. Proxies audio inference requests to the audiocpp_server HTTP API.

### 2. AudioC++ Client (`internal/audiocpp/`)
HTTP client that communicates with the audiocpp_server subprocess. Provides typed methods for all supported operations:
- Speech (TTS)
- Transcription (ASR)
- Health checks
- Model listing
- Voice listing
- Generic task execution
- CLI executor fallback

### 3. Process Manager (`internal/audiocpp/process.go`)
Manages the lifecycle of the audiocpp_server subprocess:
- Start with JSON config generation
- Health monitoring and auto-restart
- Graceful shutdown
- Output capture

### 4. Runtime (`internal/runtime/`)
Orchestrates all components. Initializes storage, client, process, workers, and manages the full lifecycle.

### 5. Job System (`internal/jobs/`)
- Job model with priority, status tracking
- Priority heap-based queue
- Manager with CRUD operations
- Worker pool processing jobs concurrently

### 6. Model Registry (`internal/models/`)
- Manifest-based model metadata
- JSON file persistence
- Auto-refresh from audiocpp_server
- Capability and language indexing

### 7. Storage (`internal/storage/`)
- SQLite database (WAL mode)
- Migrations system
- Jobs and outputs repositories

### 8. Outputs Manager (`internal/outputs/`)
- File storage for generated audio
- MIME type to extension mapping
- Retention-based cleanup

## Data Flow: TTS Request

```
Client → API Server (POST /v1/tts)
  → Model Registry lookup (validate model exists)
  → AudioC++ Client.Speech()
    → HTTP POST to audiocpp_server /v1/audio/speech
    → Return audio bytes
  → Response to client (audio/wav)
```

## Data Flow: ASR Request (Multipart)

```
Client → API Server (POST /v1/asr, multipart)
  → Parse multipart form
  → Save uploaded file to temp directory
  → AudioC++ Client.TranscribeMultipart()
    → HTTP POST to audiocpp_server /v1/audio/transcriptions
    → Return transcription JSON
  → Cleanup temp file
  → Response to client (application/json)
```

## Data Flow: Job Processing

```
Client → API Server (POST /v1/jobs)
  → Create job record in DB (status: pending)
  → Enqueue in priority queue (status: queued)
  → Worker picks up job (status: running)
  → Worker executes via AudioC++ Client
  → Save result or error (status: completed/failed)
  → Outputs stored via Outputs Manager
```
