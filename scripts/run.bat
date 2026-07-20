@echo off
setlocal enabledelayedexpansion

set "ROOT_DIR=%~dp0.."
cd /d "%ROOT_DIR%"

if not exist "bin\audiocpp-runtime.exe" (
    echo Building first...
    call scripts\build.bat
    if !ERRORLEVEL! neq 0 exit /b 1
)

echo Starting audiocpp-runtime...
bin\audiocpp-runtime.exe %*
