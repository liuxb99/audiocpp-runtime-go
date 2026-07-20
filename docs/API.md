# API Reference

## Base URL

All API endpoints are served under `/v1/`.

Default: `http://127.0.0.1:8091/v1/`

## Endpoints

### Health Check

```
GET /v1/health
```

Response:
```json
{
    "status": "ok",
    "version": "1.0.0",
    "audiocpp_alive": true,
    "models_count": 2,
    "jobs_pending": 0,
    "jobs_running": 0,
    "uptime_seconds": 123.4
}
```

### List Models

```
GET /v1/models
```

Response:
```json
{
    "data": [
        {
            "id": "pocket-tts",
            "name": "pocket-tts",
            "family": "pocket_tts",
            "task": "tts",
            "path": "pocket-tts",
            "capabilities": ["tts"],
            "created_at": "...",
            "updated_at": "..."
        }
    ]
}
```

### Get Model

```
GET /v1/models/{id}
```

### Text-to-Speech (Internal API)

```
POST /v1/tts
```

Request:
```json
{
    "model": "pocket-tts",
    "input": "Hello world",
    "voice": "alba",
    "language": "en",
    "response_format": "wav",
    "seed": 42,
    "temperature": 0.7,
    "top_k": 50,
    "top_p": 0.9
}
```

Response: Raw audio bytes (Content-Type: audio/wav)

### Text-to-Speech (OpenAI-compatible)

```
POST /v1/audio/speech
```

Same as `/v1/tts` — accepts OpenAI-compatible request format and proxies to audiocpp_server.

### Automatic Speech Recognition (Internal API)

```
POST /v1/asr
```

Supports both JSON and multipart/form-data.

JSON:
```json
{
    "model": "whisper",
    "audio": "/path/to/audio.wav",
    "language": "en"
}
```

Multipart:
- `model`: Model ID
- `file`: Audio file (WAV)
- `language`: Optional language hint

### Automatic Speech Recognition (OpenAI-compatible)

```
POST /v1/audio/transcriptions
```

Same as `/v1/asr` — OpenAI-compatible proxy endpoint.

### Generic Task

```
POST /v1/tasks/run
POST /v1/tasks/stream
```

Request:
```json
{
    "model": "model-id",
    "request": { ... task-specific fields ... }
}
```

### List Voices

```
GET /v1/voices?model=<id>
```

Response:
```json
{
    "voices": ["alba", "cosette"]
}
```

### Capabilities

```
GET /v1/capabilities
```

### Jobs

```
POST /v1/jobs
```

Request:
```json
{
    "type": "tts",
    "model_id": "pocket-tts",
    "request": { "input": "Hello" },
    "priority": 0
}
```

```
GET /v1/jobs[?status=<status>&limit=<n>&offset=<n>]
GET /v1/jobs/{id}
POST /v1/jobs/{id}/cancel
GET /v1/jobs/{id}/outputs
```

### Outputs

```
GET /v1/outputs/{id}
```
