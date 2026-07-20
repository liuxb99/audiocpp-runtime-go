# Real audio.cpp Build (Windows CPU)

## Build Environment

| Field | Value |
|---|---|
| OS | Windows 11 |
| Compiler | MSVC (Visual Studio Community 2026, v18) |
| CMake | 4.3.3 |
| Go | 1.22.12 |
| Architecture | x64 |
| Backend | CPU (no CUDA) |
| OpenMP | MSVC `/openmp:experimental` |

## CMake Configuration

```
cmake -S . -B build/windows-cpu-release ^
  -G Ninja ^
  -DCMAKE_BUILD_TYPE=Release ^
  -DCMAKE_C_COMPILER=cl.exe ^
  -DCMAKE_CXX_COMPILER=cl.exe ^
  -DCMAKE_C_FLAGS=/utf-8 ^
  -DCMAKE_CXX_FLAGS=/utf-8 /EHsc ^
  -DCMAKE_MAKE_PROGRAM=ninja.exe ^
  -DOpenMP_C_FLAGS=/openmp:experimental ^
  -DOpenMP_CXX_FLAGS=/openmp:experimental ^
  -DENGINE_ENABLE_CUDA=OFF ^
  -DENGINE_ENABLE_OPENMP=ON ^
  -DENGINE_ENABLE_CUDA_GRAPHS=OFF ^
  -DENGINE_ENABLE_VULKAN=OFF ^
  -DENGINE_ENABLE_METAL=OFF ^
  -DGGML_OPENMP=ON ^
  -DENGINE_ENABLE_NATIVE_CPU=ON ^
  -DENGINE_ENABLE_LLAMAFILE=ON ^
  -DENGINE_BUILD_TESTS=OFF ^
  -DAUDIOCPP_DEPLOYMENT_BUILD=OFF
```

## Build Command

```powershell
.\scripts\build_windows.ps1 -Preset windows-cpu-release -Target audiocpp_server -Jobs 16
```

## Output Binary

| Field | Value |
|---|---|
| Path | `audio.cpp/build/windows-cpu-release/bin/audiocpp_server.exe` |
| Size | 10,211,328 bytes (9.7 MB) |
| Type | CPU-only, Release, MSVC |

## Server Dependencies

The server links `engine_runtime.lib` (static) which includes ggml, sentencepiece, cJSON, and libyaml.
On Windows it also links `ws2_32` for socket support.

## Known Issues

- **lazy_load**: Setting `"lazy_load": true` (top-level) causes model spec override to not be applied during model loading. Always set `"lazy_load": false` or per-model `"lazy": false` when using `model_spec_override`.
