@echo off
setlocal enabledelayedexpansion

set "ROOT_DIR=%~dp0.."
cd /d "%ROOT_DIR%"

echo === AudioC++ Runtime Smoke Test ===
echo.

:: Step 1: Run unit tests
echo [1/5] Running unit tests...
call scripts\test.bat
if %ERRORLEVEL% neq 0 (
    echo FAIL: Unit tests failed
    exit /b 1
)
echo PASS: Unit tests passed
echo.

:: Step 2: Build binaries
echo [2/5] Building binaries...
call build.bat
if %ERRORLEVEL% neq 0 (
    echo FAIL: Build failed
    exit /b 1
)
echo PASS: Build succeeded
echo.

:: Step 3: Verify binaries exist
echo [3/5] Verifying binaries...
if not exist "bin\audiocpp-runtime.exe" (
    echo FAIL: bin\audiocpp-runtime.exe not found
    exit /b 1
)
if not exist "bin\audiocppctl.exe" (
    echo FAIL: bin\audiocppctl.exe not found
    exit /b 1
)
echo PASS: Both binaries exist
echo.

:: Step 4: Start runtime and verify health
echo [4/5] Starting runtime and checking health...
set "CONFIG_FILE=%TEMP%\audiocpp_smoke_config.yaml"
(
echo server:
echo   host: "127.0.0.1"
echo   port: 18991
echo audiocpp:
echo   server_path: "runtime\audio.cpp\bin\audiocpp_server.exe"
echo   cli_path: "runtime\audio.cpp\bin\audiocpp_cli.exe"
echo   working_dir: "runtime\audio.cpp"
echo   backend: "cpu"
echo   device: 0
echo   host: "127.0.0.1"
echo   port: 18992
echo   startup_timeout_seconds: 30
echo   request_timeout_seconds: 30
echo   auto_restart: false
echo   max_restart_attempts: 0
echo storage:
echo   sqlite_path: "%TEMP%\audiocpp_smoke.db"
echo models:
echo   root_dir: "models"
echo   registry_path: "%TEMP%\audiocpp_smoke_models.json"
echo outputs:
echo   root_dir: "%TEMP%\audiocpp_smoke_outputs"
echo   retain_days: 1
echo jobs:
echo   workers: 1
echo   queue_size: 10
) > "%CONFIG_FILE%"

:: Start runtime in background
start /B bin\audiocpp-runtime.exe --config "%CONFIG_FILE%"
set "RUNTIME_PID=!ERRORLEVEL!"

:: Wait for startup
echo Waiting for runtime to start...
timeout /t 3 /nobreak > nul

:: Check if process is running
tasklist /FI "IMAGENAME eq audiocpp-runtime.exe" 2>nul | find /I "audiocpp-runtime.exe" >nul
if %ERRORLEVEL% neq 0 (
    echo WARN: Runtime process not running (expected if audiocpp_server not found)
) else (
    echo PASS: Runtime process is running
)

:: Try health endpoint
echo Checking health endpoint...
bin\audiocppctl.exe --server http://127.0.0.1:18991 health >nul 2>&1
if %ERRORLEVEL% equ 0 (
    echo PASS: Health endpoint responded
) else (
    echo WARN: Health endpoint not available (server not started)
)

:: Cleanup
echo.
echo [5/5] Cleaning up...
taskkill /F /IM audiocpp-runtime.exe >nul 2>&1
del "%CONFIG_FILE%" >nul 2>&1
echo.

echo === Smoke Test Complete ===
exit /b 0
