# Real Model: Citrinet ASR (Citrinet-256)

## Model Info

| Field | Value |
|---|---|
| Name | Citrinet-256 |
| Family | `citrinet_asr` |
| Task | ASR (Automatic Speech Recognition) |
| Mode | `offline` |
| Language | English |
| Parameters | ~14M |
| Format | safetensors (converted from NVIDIA NeMo .nemo archive) |

## Files and Checksums

> SHA256 values are recorded at model install time on each machine.

| File | Size | SHA256 |
|------|------|--------|
| `citrinet_256.safetensors` | 41,486,364 bytes | *(record at install time)* |
| `citrinet_256_config.json` | 6,825 bytes | *(record at install time)* |
| `citrinet_256_tokenizer.model` | 253,072 bytes | *(record at install time)* |
| `citrinet_256_vocab.txt` | 5,519 bytes | *(record at install time)* |

To record SHA256 after model installation:

```powershell
Get-FileHash audio.cpp/models/citrinet/citrinet_256.safetensors -Algorithm SHA256
Get-FileHash audio.cpp/models/citrinet/citrinet_256_config.json -Algorithm SHA256
Get-FileHash audio.cpp/models/citrinet/citrinet_256_tokenizer.model -Algorithm SHA256
```

## Source

| Field | Value |
|---|---|
| Source format | NVIDIA NeMo archive (`.nemo`) |
| Download URL | https://api.ngc.nvidia.com/v2/models/nvidia/nemo/stt_en_citrinet_256/versions/1.0.0rc1/files/stt_en_citrinet_256.nemo |
| Install tool | `audio.cpp/tools/model_manager.py install citrinet_asr` |
| Install target | `audio.cpp/models/citrinet/` |

## Server Config

```json
{
  "model_spec_override": "<repo-root>/audio.cpp/model_specs",
  "models": [{
    "id": "citrinet-asr",
    "family": "citrinet_asr",
    "path": "<repo-root>/audio.cpp/models/citrinet",
    "task": "asr",
    "mode": "offline"
  }]
}
```

In `configs/config.example.yaml`:

```yaml
audiocpp:
  lazy_load: false
  model_spec_override: ./audio.cpp/model_specs
  models:
    - id: citrinet-asr
      path: ./audio.cpp/models/citrinet
      family: citrinet_asr
```

## API

```http
POST /v1/audio/transcriptions
Content-Type: multipart/form-data

file=@testdata/audio/english_short_16k.wav
model=citrinet-asr
```

## Smoke Test

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/real_smoke_test.ps1
```

## Performance

~90ms wall time for 3s audio at 16kHz on CPU (0.03 RTF)
