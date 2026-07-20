@echo off
setlocal enabledelayedexpansion

set "ROOT_DIR=%~dp0.."
cd /d "%ROOT_DIR%"

echo Running tests...
go test -v -count=1 ./...
if %ERRORLEVEL% neq 0 (
    echo ERROR: tests failed
    exit /b 1
)
echo All tests passed.
