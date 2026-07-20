@echo off
setlocal enabledelayedexpansion

set "ROOT_DIR=%~dp0.."
cd /d "%ROOT_DIR%"

echo Building audiocpp-runtime-go...

if not exist "bin" mkdir bin

go build -ldflags="-s -w" -o bin\audiocpp-runtime.exe .\cmd\audiocpp-runtime\
if %ERRORLEVEL% neq 0 (
    echo ERROR: audiocpp-runtime build failed
    exit /b 1
)
echo Built: bin\audiocpp-runtime.exe

go build -ldflags="-s -w" -o bin\audiocppctl.exe .\cmd\audiocppctl\
if %ERRORLEVEL% neq 0 (
    echo ERROR: audiocppctl build failed
    exit /b 1
)
echo Built: bin\audiocppctl.exe

echo Build complete.
