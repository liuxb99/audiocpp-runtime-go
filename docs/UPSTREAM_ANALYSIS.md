# Upstream Analysis: audio.cpp

## Repository Structure

- **audio.cpp/** — Core C++ inference framework
  - `app/server/` — HTTP server source
  - `app/cli/` — CLI source
  - `src/` — engine_runtime library
  - `external/` — ggml, cJSON, libyaml, sentencepiece, llama_tokenizer
- **audio.cpp-webui/** — Downstream distribution with Gradio web UI

## Build System (CMakeLists.txt)

### Targets

| Target | Description |
|--------|-------------|
| `engine_runtime` | Static library with all framework/model code |
| `audiocpp_server` | HTTP server binary |
| `audiocpp_cli` | CLI binary |
| `audiocpp_gguf` | GGUF converter tool |
| `model_perf` | Model performance testing |

### CMake Options

| Option | Default | Description |
|--------|---------|-------------|
| `ENGINE_ENABLE_CUDA` | OFF (ON if GGML_CUDA) | CUDA backend |
| `ENGINE_ENABLE_VULKAN` | OFF (ON if GGML_VULKAN) | Vulkan backend |
| `ENGINE_ENABLE_METAL` | ON on APPLE | Metal backend |
| `ENGINE_ENABLE_LLAMAFILE` | ON | llamafile SGEMM |
| `ENGINE_ENABLE_NATIVE_CPU` | ON | Native host ISA |
| `ENGINE_ENABLE_OPENMP` | ON | OpenMP support |
| `AUDIOCPP_DEPLOYMENT_BUILD` | OFF | Compile model specs into runtime |

### Dependencies (external)

- **ggml** — subtree in `external/ggml`
- **sentencepiece** — tokenizer
- **cJSON** — JSON parsing
- **libyaml** — YAML parsing
- **llama_tokenizer** — tokenizer
- **ws2_32** — Windows Winsock

## Server Startup

### Command-Line Arguments

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--config <path>` | string | **YES** | — | Path to server config JSON |
| `--host <ip>` | string | no | from config (127.0.0.1) | Listen IPv4 address |
| `--port <port>` | int | no | from config (8080) | Listen port |
| `--backend <backend>` | string | no | from config (cuda) | cpu/cuda/vulkan/metal |
| `--device <id>` | int | no | from config (0) | GPU device ID |
| `--threads <n>` | int | no | from config (1) | Thread count |
| `--model-spec-override` | string | no | — | Model spec resolution |
| `--log` | flag | no | off | Enable framework logging |
| `--log-file <path>` | string | no | — | Log to file |

### Help Text

```
audiocpp_server --config <server.json> [--host <ip>] [--port <port>] [--backend <backend>]
                [--device <id>] [--threads <n>]
                [--model-spec-override <json-or-directory>]
                [--log] [--log-file <path>]
  --backend cpu|cuda|vulkan|metal  default cuda
```

### Startup Sequence

1. Parse CLI args
2. Configure logging
3. Register SIGINT/SIGTERM signal handlers
4. Load JSON config file
5. Apply CLI overrides on config
6. Create ServerState — loads all models (or marks them lazy)
7. Serve HTTP — raw POSIX/Winsock sockets with select() polling
8. On shutdown: print "audiocpp_server stopped"

## Server Configuration

### ServerConfig

```json
{
    "host": "127.0.0.1",
    "port": 8080,
    "backend": "cuda",
    "device": 0,
    "threads": 1,
    "lazy_load": false,
    "models": [
        {
            "id": "pocket-tts",
            "path": "models/pocket-tts",
            "family": "pocket_tts",
            "task": "tts",
            "mode": "offline",
            "lazy": false,
            "voice_presets": {
                "alba": {
                    "voice_id": "alba",
                    "voice_ref": "voices/alba.wav",
                    "reference_text": "reference transcript"
                }
            }
        }
    ]
}
```

### ServerModelConfig Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | YES | Unique model identifier, used in API as "model" |
| `path` | string | YES | Path to model directory (relative to config file) |
| `family` | string | YES | Model family (pocket_tts, qwen3_asr, etc.) |
| `task` | string | no (tts) | tts, asr, vad, etc. |
| `mode` | string | no (offline) | offline, streaming |
| `lazy` | bool | no | Defer model load |
| `model_spec_override` | string | no | Override spec path |
| `config` | string | no | Config selector |
| `weight` | string | no | Weight selector |
| `load_options` | object | no | Load options KV pairs |
| `session_options` | object | no | Session options KV pairs |
| `voice_presets` | object | no | Named voice presets |
| `default_voice_preset` | string/object | no | Default voice preset |

**Backend default is CUDA** (not CPU like the CLI).

## Complete Route Table

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/health` | inline | Health check |
| GET | `/v1/models` | `models_json()` | List available models |
| GET | `/v1/audio/voices` | `handle_voices()` | List voices for model |
| POST | `/v1/audio/speech` | `handle_speech()` | Text-to-Speech |
| POST | `/v1/audio/transcriptions` | `handle_transcription()` | Speech-to-Text |
| POST | `/v1/tasks/run` | `handle_generic_run()` | Generic offline task |
| POST | `/v1/tasks/stream` | `handle_generic_stream()` | Generic streaming task |
| any | anything else | — | 404 not_found |

## Endpoint Details

### GET /health

```json
{"status":"ok","backend":"cuda","models":2}
```

### GET /v1/models

```json
{
    "object": "list",
    "data": [
        {"id":"pocket-tts","object":"model","owned_by":"engine","family":"pocket_tts","task":"tts","mode":"offline"}
    ]
}
```

### GET /v1/audio/voices?model=<id>

Returns voice names from configured voice_presets + .safetensors files in `<model_path>/embeddings/`.

```json
{"voices":["alba","cosette","some_embedding_id"]}
```

### POST /v1/audio/speech (TTS)

**Request body** (JSON):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | string | YES | Model ID from server config |
| `input` | string | YES | Text to synthesize |
| `voice` | string | no | Voice ID or preset name |
| `language` | string | no | Language code |
| `response_format` | string | no | wav, json, b64_json |
| `stream` | bool | no | Enable streaming |
| `stream_format` | string | no | sse, audio |
| `voice_ref` | string | no | Reference WAV path |
| `reference_text` | string | no | Reference transcript |
| `instructions` | string | no | Voice design instruction |
| `seed` | int | no | Random seed |
| `temperature` | float | no | Sampling temperature |
| `top_k` | int | no | Top-k sampling |
| `top_p` | float | no | Top-p sampling |
| `max_tokens` | int | no | Max output tokens |
| `max_steps` | int | no | Max inference steps |
| `repetition_penalty` | float | no | Repetition penalty |
| `guidance_scale` | float | no | CFG scale |
| `num_inference_steps` | int | no | Diffusion steps |
| `options` | object | no | Model-specific options |

**Non-streaming responses:**
- `response_format=wav`: `Content-Type: audio/wav`, raw WAV binary, headers: `X-AudioCPP-Wall-Ms`, `X-AudioCPP-Audio-Duration-Ms`, `X-AudioCPP-RTF`
- `response_format=json|b64_json`: `{"audio":"<base64>","format":"wav","timing":{"wall_ms":...,"audio_duration_ms":...,"rtf":...}}`

**Streaming SSE events:**
```
data: {"type":"speech.audio.delta","audio":"<base64-pcm16>"}
data: {"type":"speech.audio.done","timing":{"ttft_ms":123.4}}
data: [DONE]
```

### POST /v1/audio/transcriptions (ASR)

**Input format 1**: JSON body with `{model, audio|audio_path|file, language?, text?, stream?, options?}`

**Input format 2**: multipart/form-data (OpenAI Whisper API compatible):
- `file` — WAV upload (only WAV supported; MP3 planned)
- `model` — Model ID
- `language` — Language hint
- `stream` — "true"/"false"

**Non-streaming response:** `{"text":"...","timing":{"wall_ms":...,"audio_duration_ms":...,"rtf":...}}`

**Streaming SSE:**
```
data: {"type":"transcript.text.delta","delta":"partial text"}
data: {"type":"transcript.text.done","text":"full text","timing":{"ttft_ms":123.4}}
data: [DONE]
```

### POST /v1/tasks/run

Generic offline task execution. Body: `{"model":"...","request":{...}}` or flat TaskRequest.

**Response** includes: text, audio (base64-wav), sample_rate, channels, named_audio_outputs[], segments[], speaker_turns[], words[], timing.

### POST /v1/tasks/stream

Generic streaming task. Returns all accumulated events + final result.

## Supported Model Families

| Family | Tasks | Description |
|--------|-------|-------------|
| pocket_tts | TTS | Lightweight TTS with voice cloning |
| miocodec | Audio codec | Audio compression/decompression |
| miotts | TTS | TTS |
| moss_tts_local | TTS | Moss TTS local |
| moss_tts_nano | TTS | Moss TTS nano |
| voxcpm2 | TTS | VoxCPM2 TTS |
| vibevoice | TTS | VibeVoice TTS |
| vibevoice_asr | ASR | VibeVoice ASR |
| qwen3_tts | TTS | Qwen3 TTS |
| qwen3_asr | ASR | Qwen3 ASR |
| qwen3_forced_aligner | Alignment | Forced alignment |
| index_tts2 | TTS | IndexTTS2 |
| irodori_tts | TTS | Irodori TTS |
| heartmula | TTS | Heartmula TTS |
| hviske_asr | ASR | Hviske ASR |
| higgs_audio_stt | ASR | Higgs Audio STT |
| nemotron_asr | ASR | Nemotron ASR |
| chatterbox | TTS/VC | Chatterbox TTS/VC |
| seed_vc | VC | Seed Voice Conversion |
| vevo2 | Gen | Vevo2 generation |
| stable_audio | Gen | Stable Audio generation |
| ace_step | Gen | ACE Step generation |
| supertonic | Gen | Supertonic generation |
| sortformer_diar | Diarization | Speaker diarization |
| silero_vad | VAD | Voice activity detection |
| citrinet_asr | ASR | Citrinet ASR |
| marblenet_vad | VAD | MarbleNet VAD |
| demucs | Source separation | Music source separation |
| roformer | Source separation | RoFormer separation |
| omnivoice | TTS/ASR | OmniVoice |
| seed_vc | VC | Voice conversion |

## HTTP Implementation Details

- **Raw sockets** (POSIX or Winsock), no external HTTP library
- **No thread pool** — each connection gets `std::thread(...).detach()`
- **Per-model mutex** — models locked during execution
- **No request queue** — simultaneous requests to same model block on mutex
- **Single listening thread** with select() polling (250ms interval)
- **No HTTPS, no authentication**, no API keys
- **IPv4 only** — host must be valid IPv4 address (inet_pton check)
- **Config paths** resolved relative to config file parent directory
- **voice_ref** paths resolved relative to server CWD
- **WAV-only** uploads for multipart transcription

## CLI (audiocpp_cli)

```
audiocpp_cli --task <task> --family <family> --model <path> [options]
```

Key flags: `--task`, `--family`, `--model`, `--backend`, `--device`, `--text`, `--audio`, `--voice-ref`, `--reference-text`, `--seed`, `--language`, `--out`, `--threads`

CLI defaults to **CPU** backend (server defaults to CUDA).

## WebUI (audio.cpp-webui)

The WebUI manages an external `audiocpp_server.exe` subprocess:
1. Dynamically writes a single-model JSON config to temp file
2. Launches `audiocpp_server.exe --config <temp.json> --host ... --port ... --device ...`
3. Proxies requests via HTTP
4. `_env.bat` script auto-detects CUDA vs CPU

The WebUI also supports WebSocket-based realtime voice chat (`ws://.../v1/realtime`).

## Integration Points

| Integration | Method | Details |
|-------------|--------|---------|
| Primary | HTTP API | REST calls to audiocpp_server |
| Fallback | CLI subprocess | audiocpp_cli when server unavailable |
| Process Mgmt | Start/Stop/Monitor | Child process lifecycle |
| Model Discovery | Query server | GET /v1/models |
| Config | JSON file | Written dynamically, path passed via --config |
