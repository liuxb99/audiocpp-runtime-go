# Real ASR Smoke Test Result

| Field | Value |
|-------|-------|
| Execution Time | 2026-07-23 07:29:14 |
| Total Duration | 8478ms |
| Git Commit | d9468fad679813dc8ee902063bcd5a298ae62a7a |
| Go Runtime Binary | D:\AI-Tools\audio-cpp-runtime-go\bin\audiocpp-runtime.exe |
| audiocpp_server Binary | D:\AI-Tools\audio-cpp-runtime-go\audio.cpp\build\windows-cpu-release\bin\audiocpp_server.exe |
| audio.cpp Upstream SHA | cd91110b39ad48cdb594d893687e9d2ae8ce0dbf |
| Citrinet Model SHA256 | {"citrinet_256.safetensors":"15AA3209AE85DE5197685D61A676BE66CD736F1448E712E3BC826F175B0EF810","citrinet_256_config.json":"2FD0FB838B92917DA5B0852885463D5FA0DFB5D2DC0415D7E5AEAE1F17631789","citrinet_256_tokenizer.model":"A7DBB4EAC08E0E7713D23C0E1B18C0A52E2278C540186728A249B77DC63D1177"} |
| Input WAV SHA256 | C60AE0337844C9B4071E242DA4F916EF7113BE0AE90AF27971152879653EBC93 |
| Runtime PID | 5952 |
| Child PID | 25796 |
| Request URL | POST http://127.0.0.1:18091/v1/audio/transcriptions |
| HTTP Status | 200 |
| Transcription | the quickbrown fox jumps over the lazy blog |
| Expected Transcription | The quick brown fox jumps over the lazy dog |
| Match Result | 6/9 words matched (66.7%) |
| Inference Duration | 154ms |
| Shutdown Duration | 5326ms |
| Graceful Shutdown | True |
| Force Kill Used | True |
| Runtime Exited Cleanly | True |
| Child Exited Cleanly | True |
| Response Received | True |
| Response Parsed | True |
| Runtime Port Free | True |
| AudioCpp Port Free | True |

## Verdict

**REAL_SMOKE_PASS**