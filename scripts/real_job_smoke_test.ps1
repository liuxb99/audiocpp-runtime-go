<#
.SYNOPSIS
    Real ASR Job Smoke Test — end-to-end verification via Job API.
.DESCRIPTION
    Steps:
     1. Check prerequisites
     2. Generate temporary smoke config
     3. Start Go Runtime
     4. Wait for health (status=ok, state=running)
     5. Create ASR Job via POST /v1/jobs
     6. Poll GET /v1/jobs/{id} until terminal status
     7. Verify result fields
     8. Output REAL_JOB_SMOKE_PASS or REAL_JOB_SMOKE_FAIL
     9. Cleanup: send POST /v1/shutdown
#>

param(
    [string]$RuntimeExe = "bin/audiocpp-runtime.exe",
    [string]$TestWav = "testdata/audio/english_short_16k.wav",
    [string]$ModelDir = "audio.cpp/models/citrinet",
    [string]$ModelSpecDir = "audio.cpp/model_specs",
    [string]$ServerExe = "audio.cpp/build/windows-cpu-release/bin/audiocpp_server.exe",
    [string]$ArtifactsDir = "artifacts/smoke",
    [int]$RuntimePort = 18091,
    [int]$AudioCppPort = 18092,
    [int]$ReadyTimeoutSec = 120,
    [int]$JobPollTimeoutSec = 120
)

$ErrorActionPreference = "Stop"
$startTime = Get-Date

# ---- Resolve repo root ----
$scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptPath "..")
Set-Location $repoRoot
Write-Host "[smoke] Repo root: $repoRoot"

function Write-Log($msg) {
    Write-Host "[smoke] $(Get-Date -Format 'HH:mm:ss') $msg"
}

function Write-FileUtf8NoBom {
    param(
        [Parameter(Mandatory, Position=0)]
        [string]$path,
        [Parameter(ValueFromPipeline)]
        [string]$content
    )
    process {
        $utf8NoBom = New-Object System.Text.UTF8Encoding $false
        [System.IO.File]::WriteAllText($path, $content, $utf8NoBom)
    }
}

Write-Log "=== REAL JOB SMOKE TEST START ==="

# ---- 1. Check prerequisites ----
$runtimePath = Join-Path $repoRoot $RuntimeExe
if (-not (Test-Path $runtimePath)) {
    Write-Host "REAL_JOB_SMOKE_FAIL: Go Runtime binary not found at $runtimePath"
    exit 1
}
Write-Log "[1] Go Runtime binary: $runtimePath"

$serverPath = Join-Path $repoRoot $ServerExe
if (-not (Test-Path $serverPath)) {
    Write-Host "REAL_JOB_SMOKE_FAIL: audiocpp_server.exe not found at $serverPath"
    exit 1
}
Write-Log "[1] audiocpp_server.exe: $serverPath"

$wavPath = Join-Path $repoRoot $TestWav
if (-not (Test-Path $wavPath)) {
    Write-Host "REAL_JOB_SMOKE_FAIL: Test WAV not found at $wavPath"
    exit 1
}
Write-Log "[1] Test WAV: $wavPath ($((Get-Item $wavPath).Length) bytes)"

$modelPath = Join-Path $repoRoot $ModelDir
$requiredModels = @("citrinet_256.safetensors", "citrinet_256_config.json", "citrinet_256_tokenizer.model", "citrinet_256_vocab.txt")
foreach ($f in $requiredModels) {
    $fp = Join-Path $modelPath $f
    if (-not (Test-Path $fp)) {
        Write-Host "REAL_JOB_SMOKE_FAIL: Model file missing: $fp"
        exit 1
    }
}
Write-Log "[1] Model files present"

# ---- 2. Generate temporary smoke config ----
$smokeConfig = @"
server:
  host: "127.0.0.1"
  port: $RuntimePort
audiocpp:
  server_path: "$($serverPath -replace '\\', '/')"
  working_dir: "$((Join-Path $repoRoot 'audio.cpp') -replace '\\', '/')"
  backend: "cpu"
  host: "127.0.0.1"
  port: $AudioCppPort
  startup_timeout_seconds: $ReadyTimeoutSec
  request_timeout_seconds: 600
  auto_restart: false
  max_restart_attempts: 0
  lazy_load: false
  model_spec_override: "$((Join-Path $repoRoot $ModelSpecDir) -replace '\\', '/')"
  models:
    - id: "citrinet_256"
      path: "$((Join-Path $repoRoot $ModelDir) -replace '\\', '/')"
      family: "citrinet_asr"
      task: "asr"
      mode: "offline"
storage:
  sqlite_path: "data/runtime-job-smoke.db"
models:
  root_dir: "models"
  registry_path: "data/models-job-smoke.json"
outputs:
  root_dir: "outputs"
  retain_days: 1
jobs:
  workers: 1
  queue_size: 10
  default_timeout_seconds: 300
  max_attempts: 1
"@
$smokeConfigPath = Join-Path $env:TEMP "audiocpp_job_smoke_config_$(Get-Random).yaml"
Write-FileUtf8NoBom -path $smokeConfigPath -content $smokeConfig
Write-Log "[2] Generated smoke config: $smokeConfigPath"

# Save config artifact
Copy-Item $smokeConfigPath (Join-Path $ArtifactsDir "smoke_config.yaml") -Force

# ---- 3. Start Go Runtime ----
$runtimeLog = Join-Path $ArtifactsDir "runtime_job_smoke.log"
$procRuntime = Start-Process -FilePath $runtimePath -ArgumentList "-config", $smokeConfigPath -NoNewWindow -PassThru -RedirectStandardOutput $runtimeLog -RedirectStandardError "$($ArtifactsDir)/runtime_job_smoke_stderr.log"
$runtimePID = $procRuntime.Id
Write-Log "[3] Started Go Runtime (PID: $runtimePID)"

# Wait a moment for startup
Start-Sleep -Seconds 3

# ---- 4. Wait for Go Runtime ready ----
$healthUrl = "http://127.0.0.1:$RuntimePort/v1/health"
$ready = $false
$timeout = [datetime]::Now.AddSeconds($ReadyTimeoutSec)
while ([datetime]::Now -lt $timeout) {
    try {
        $resp = Invoke-RestMethod -Uri $healthUrl -Method Get -ErrorAction Stop
        $healthData = if ($resp.data) { $resp.data } else { $resp }
        $healthStatus = $healthData.status
        $audiocppAlive = $healthData.audiocpp_alive
        $audiocppState = $healthData.audiocpp_state
        $runtimeState = $healthData.runtime_state
        Write-Log "[4] Health: status=$healthStatus audiocpp_alive=$audiocppAlive state=$audiocppState runtime=$runtimeState"
        if ($healthStatus -eq "ok" -and $audiocppAlive -eq $true -and $audiocppState -eq "running") {
            $ready = $true
            Write-Log "[4] Go Runtime ready"
            break
        }
    } catch {
        # Not ready yet
    }
    Start-Sleep -Milliseconds 1000
}
if (-not $ready) {
    Write-Host "REAL_JOB_SMOKE_FAIL: Go Runtime did not become ready within ${ReadyTimeoutSec}s"
    # Get log tail for diagnostics
    if (Test-Path $runtimeLog) {
        Write-Log "[diag] Runtime log tail:"
        Get-Content $runtimeLog -Tail 50
    }
    Stop-Process -Id $runtimePID -Force -ErrorAction SilentlyContinue
    exit 1
}

# ---- 5. Create ASR Job ----
$jobUrl = "http://127.0.0.1:$RuntimePort/v1/jobs"
$jobBody = @{
    type = "asr"
    model_id = "citrinet_256"
    request = @{
        audio_path = $TestWav
        language = "en"
    }
} | ConvertTo-Json -Depth 5
Write-Log "[5] Creating job: $jobBody"

$jobResponse = $null
$jobId = $null
try {
    $jobResponseRaw = Invoke-RestMethod -Uri $jobUrl -Method Post -Body $jobBody -ContentType "application/json" -ErrorAction Stop
    # API wraps response in { "data": ... }
    $jobData = if ($jobResponseRaw.data) { $jobResponseRaw.data } else { $jobResponseRaw }
    $jobId = $jobData.id
    $jobStatus = $jobData.status
    Write-Log "[5] Job created: id=$jobId status=$jobStatus"
    
    # Save response
    $jobData | ConvertTo-Json -Depth 10 | Write-FileUtf8NoBom -path (Join-Path $ArtifactsDir "job_create_response.json")
} catch {
    Write-Log "[5] Job creation FAILED: $_"
    # Try to get more info from the error
    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $errorBody = $reader.ReadToEnd()
        Write-Log "[5] Error response body: $errorBody"
        $errorBody | Write-FileUtf8NoBom -path (Join-Path $ArtifactsDir "job_create_error.json")
    }
    Write-Host "REAL_JOB_SMOKE_FAIL: Job creation failed"
    Stop-Process -Id $runtimePID -Force -ErrorAction SilentlyContinue
    exit 1
}

# ---- 6. Poll job status ----
$finalStatus = $null
$pollStart = Get-Date
$pollTimeout = [datetime]::Now.AddSeconds($JobPollTimeoutSec)
while ([datetime]::Now -lt $pollTimeout) {
    Start-Sleep -Seconds 2
    try {
        $jobStatusRaw = Invoke-RestMethod -Uri "$jobUrl/$jobId" -Method Get -ErrorAction Stop
        # API wraps response in { "data": ... }
        $jobStatusData = if ($jobStatusRaw.data) { $jobStatusRaw.data } else { $jobStatusRaw }
        $currentStatus = $jobStatusData.status
        Write-Log "[6] Job $jobId status: $currentStatus"
        
        # Save intermediate status
        $jobStatusData | ConvertTo-Json -Depth 10 | Write-FileUtf8NoBom -path (Join-Path $ArtifactsDir "job_status_$([DateTime]::Now.ToString('HHmmss')).json")
        
        if ($currentStatus -eq "succeeded" -or $currentStatus -eq "failed" -or $currentStatus -eq "canceled" -or $currentStatus -eq "timed_out") {
            $finalStatus = $currentStatus
            Write-Log "[6] Job reached terminal status: $currentStatus"
            # Save final result
            $jobStatusData | ConvertTo-Json -Depth 10 | Write-FileUtf8NoBom -path (Join-Path $ArtifactsDir "job_final_result.json")
            break
        }
    } catch {
        Write-Log "[6] Poll error: $_"
    }
}

$pollDuration = [math]::Round(((Get-Date) - $pollStart).TotalSeconds)
Write-Log "[6] Polling completed in ${pollDuration}s, final status: $finalStatus"

if (-not $finalStatus) {
    Write-Host "REAL_JOB_SMOKE_FAIL: Job did not reach terminal status within ${JobPollTimeoutSec}s"
    Stop-Process -Id $runtimePID -Force -ErrorAction SilentlyContinue
    exit 1
}

# ---- 7. Verify result ----
$jobFinal = Get-Content (Join-Path $ArtifactsDir "job_final_result.json") -Raw | ConvertFrom-Json
$verificationPassed = $false

if ($finalStatus -eq "succeeded") {
    Write-Log "[7] Job status: succeeded"
    
    # Check backend_name is at the top level of the job
    $backendName = $jobFinal.backend_name
    $backendNameCorrect = ($backendName -eq "audiocpp")
    Write-Log "[7] backend_name (top-level): $backendName"
    
    # Check result map if present (contains metadata, not transcription text)
    $hasResultMap = ($null -ne $jobFinal.result)
    if ($hasResultMap) {
        Write-Log "[7] Job result map: $($jobFinal.result | ConvertTo-Json -Compress)"
        $hasBackendInResult = ($jobFinal.result.backend_name -eq "audiocpp")
        Write-Log "[7] result.backend_name: $($jobFinal.result.backend_name)"
    }
    
    # Verify job used backend adapter (not direct audio.cpp call)
    # backend_name = "audiocpp" confirms it went through the adapter
    if ($backendNameCorrect) {
        $verificationPassed = $true
        Write-Log "[7] PASS - status=succeeded, backend_name=audiocpp (job used BackendManager)"
    } else {
        Write-Log "[7] FAIL - backend_name is '$backendName', expected 'audiocpp'"
    }
} else {
    Write-Log "[7] FAIL - job status is $finalStatus (expected succeeded)"
    Write-Log "[7] Job data: $($jobFinal | ConvertTo-Json -Depth 10)"
    if ($jobFinal.error) { Write-Log "[7] Error: $($jobFinal.error)" }
    if ($jobFinal.error_message) { Write-Log "[7] ErrorMessage: $($jobFinal.error_message)" }
    if ($jobFinal.error_code) { Write-Log "[7] ErrorCode: $($jobFinal.error_code)" }
}

# ---- 8. Output result ----
if ($verificationPassed) {
    Write-Host "REAL_JOB_SMOKE_PASS"
} else {
    Write-Host "REAL_JOB_SMOKE_FAIL"
}

# ---- 9. Cleanup: graceful shutdown ----
try {
    $shutdownResp = Invoke-RestMethod -Uri "http://127.0.0.1:$RuntimePort/v1/shutdown" -Method Post -TimeoutSec 10 -ErrorAction Stop
    Write-Log "[9] Shutdown API responded"
} catch {
    Write-Log "[9] Shutdown API failed: $_"
}

# Wait for process to exit
Start-Sleep -Seconds 3
if (Get-Process -Id $runtimePID -ErrorAction SilentlyContinue) {
    Write-Log "[9] Runtime still alive, force stopping"
    Stop-Process -Id $runtimePID -Force -ErrorAction SilentlyContinue
}

Write-Log "=== REAL JOB SMOKE TEST $(if ($verificationPassed) { 'PASS' } else { 'FAIL' }) ==="

if ($verificationPassed) {
    exit 0
} else {
    exit 1
}
