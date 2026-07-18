@echo off
setlocal enabledelayedexpansion

set "PROJECT_ROOT=%~dp0"
set "OUTPUT_NAME=audiocpp-runtime.exe"

echo Building audiocpp-runtime-go...
echo Project root: !PROJECT_ROOT!

cd /d "!PROJECT_ROOT!" 2>nul

echo [1/4] Cleaning old build artifacts...
if exist "!OUTPUT_NAME!" del /f /q "!OUTPUT_NAME!" 2>nul
if exist "bin" rmdir /s /q "bin" 2>nul

echo [2/4] Downloading Go dependencies...
call go mod tidy
if %errorlevel% neq 0 (
    echo ERROR: go mod tidy failed
    exit /b %errorlevel%
)

echo [3/4] Compiling Go binary (this may take a moment)...
call go build -ldflags="-s -w" -o "!OUTPUT_NAME!" .\cmd\audiocpp-runtime\
if %errorlevel% neq 0 (
    echo ERROR: Build failed
    exit /b %errorlevel%
)

echo [4/4] Verifying binary...
if exist "!OUTPUT_NAME!" (
    for %%I in ("!OUTPUT_NAME!") do echo Build complete: %%~fI (%%~zI bytes)
) else (
    echo ERROR: Binary not found after build
    exit /b 1
)

echo.
echo SUCCESS: audiocpp-runtime-go has been built.
echo Run with: !OUTPUT_NAME! --help
