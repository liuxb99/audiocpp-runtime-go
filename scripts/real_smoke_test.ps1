<#
.SYNOPSIS
    Real ASR smoke test — end-to-end verification managed by Go Runtime.
.DESCRIPTION
    Steps:
     1. Auto-locate repo root
     2. Check Go Runtime binary (audiocpp-runtime.exe)
     3. Check audiocpp_server.exe
     4. Check Citrinet model files
     5. Validate model SHA256 (recorded)
     6. Check real test WAV
     7. Generate temporary smoke config
     8. Start Go Runtime
     9. Record Runtime PID
    10. Wait for Go Runtime ready (/v1/health)
    11. Get audiocpp child PID from health endpoint
    12. POST /v1/audio/transcriptions with real WAV
    13. Save HTTP response
    14. Parse transcription text
    15. Verify text non-empty
    16. Loose match against expected text
    17. Graceful shutdown
    18. Wait for Runtime PID to exit
    19. Wait for child PID to exit
    20. Check port release
    21. Output REAL_SMOKE_PASS or REAL_SMOKE_FAIL
    22. Return correct exit code
#>

param(
    [string]$RuntimeExe = "bin/audiocpp-runtime.exe",
    [string]$ConfigTemplate = "configs/config.example.yaml",
    [string]$TestWav = "testdata/audio/english_short_16k.wav",
    [string]$ModelDir = "audio.cpp/models/citrinet",
    [string]$ModelSpecDir = "audio.cpp/model_specs",
    [string]$ServerExe = "audio.cpp/build/windows-cpu-release/bin/audiocpp_server.exe",
    [string]$ArtifactsDir = "artifacts/smoke",
    [int]$RuntimePort = 18091,
    [int]$AudioCppPort = 18092,
    [int]$ReadyTimeoutSec = 60,
    [string]$ExpectedText = "The quick brown fox jumps over the lazy dog"
)

$ErrorActionPreference = "Stop"
$startTime = Get-Date

# ---- Resolve repo root (script location) ----
$scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptPath "..")
Set-Location $repoRoot
Write-Host "[smoke] Repo root: $repoRoot"

# ---- Helper: timestamp ----
function Write-Log($msg) {
    Write-Host "[smoke] $(Get-Date -Format 'HH:mm:ss') $msg"
}

# ---- Helper: wait for PID to exit ----
function Wait-PidExit($procId, $timeoutSeconds) {
    $elapsed = 0
    while ($elapsed -lt $timeoutSeconds) {
        $proc = Get-Process -Id $procId -ErrorAction SilentlyContinue
        if (-not $proc) { return $true }
        Start-Sleep -Seconds 1
        $elapsed++
    }
    return $false
}

# ---- Helper: check port in use ----
function Test-PortFree($port) {
    # Ignore closing/transitional states — we only care about ports in established use
    $ignoreStates = @("TimeWait", "CloseWait", "FinWait1", "FinWait2", "LastAck", "Bound")
    $conn = Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue |
        Where-Object { $_.State -notin $ignoreStates }
    return (-not $conn)
}

# ---- Helper: write file as UTF-8 without BOM (PS 5.1 compatible) ----
function Write-FileUtf8NoBom($path, $content) {
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($path, $content, $utf8NoBom)
}

Write-Log "=== REAL SMOKE TEST START ==="

# ---- 1. Repo root (already set) ----
Write-Log "[1/22] Repo root: $repoRoot"

# ---- 2. Check Go Runtime binary ----
$runtimePath = Join-Path $repoRoot $RuntimeExe
if (-not (Test-Path $runtimePath)) {
    Write-Host "REAL_SMOKE_FAIL: Go Runtime binary not found at $runtimePath"
    exit 1
}
Write-Log "[2/22] Go Runtime binary: $runtimePath"

# ---- 3. Check audiocpp_server.exe ----
$serverPath = Join-Path $repoRoot $ServerExe
if (-not (Test-Path $serverPath)) {
    Write-Host "REAL_SMOKE_FAIL: audiocpp_server.exe not found at $serverPath"
    exit 1
}
Write-Log "[3/22] audiocpp_server.exe: $serverPath"

# ---- 4. Check Citrinet model files ----
$modelPath = Join-Path $repoRoot $ModelDir
$requiredModels = @("citrinet_256.safetensors", "citrinet_256_config.json", "citrinet_256_tokenizer.model")
foreach ($f in $requiredModels) {
    $fp = Join-Path $modelPath $f
    if (-not (Test-Path $fp)) {
        Write-Host "REAL_SMOKE_FAIL: Model file missing: $fp"
        exit 1
    }
}
Write-Log "[4/22] Citrinet model files present"

# ---- 5. Validate model SHA256 against recorded checksums ----
$shaJsonPath = Join-Path $repoRoot "docs/model-sha256.json"
if (-not (Test-Path $shaJsonPath)) {
    Write-Host "REAL_SMOKE_FAIL: model-sha256.json not found at $shaJsonPath"
    exit 1
}
$shaRecordedRaw = Get-Content $shaJsonPath -Raw | ConvertFrom-Json
$shaRecorded = $shaRecordedRaw.files
foreach ($f in $requiredModels) {
    $mp = Join-Path $modelPath $f
    if (-not (Test-Path $mp)) {
        Write-Host "REAL_SMOKE_FAIL: Model file missing during SHA256 check: $mp"
        exit 1
    }
    $actualHash = (Get-FileHash -Path $mp -Algorithm SHA256).Hash
    $expectedHash = $shaRecorded.$f.sha256
    if ($actualHash -ne $expectedHash) {
        Write-Host "REAL_SMOKE_FAIL: SHA256 mismatch for $f"
        Write-Host "  Expected: $expectedHash"
        Write-Host "  Actual:   $actualHash"
        exit 1
    }
    Write-Log "[5/22] $f SHA256 verified OK"
}

# ---- 6. Check real test WAV ----
$wavPath = Join-Path $repoRoot $TestWav
if (-not (Test-Path $wavPath)) {
    Write-Host "REAL_SMOKE_FAIL: Test WAV not found at $wavPath"
    exit 1
}
Write-Log "[6/22] Test WAV: $wavPath ($((Get-Item $wavPath).Length) bytes)"

# ---- 7. Generate temporary smoke config ----
$configTemplatePath = Join-Path $repoRoot $ConfigTemplate
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
    - id: "citrinet-asr"
      path: "$((Join-Path $repoRoot $ModelDir) -replace '\\', '/')"
      family: "citrinet_asr"
      task: "asr"
      mode: "offline"
storage:
  sqlite_path: "data/runtime-smoke.db"
models:
  root_dir: "models"
  registry_path: "data/models-smoke.json"
outputs:
  root_dir: "outputs"
  retain_days: 1
jobs:
  workers: 1
  queue_size: 10
"@
$smokeConfigPath = Join-Path $env:TEMP "audiocpp_smoke_config_$(Get-Random).yaml"
Write-FileUtf8NoBom -path $smokeConfigPath -content $smokeConfig
Write-Log "[7/22] Generated smoke config: $smokeConfigPath"

# ---- 8. Start Go Runtime ----
$runtimeLog = Join-Path $ArtifactsDir "runtime.log"
$procRuntime = Start-Process -FilePath $runtimePath -ArgumentList "-config", $smokeConfigPath -NoNewWindow -PassThru -RedirectStandardOutput $runtimeLog -RedirectStandardError "$($ArtifactsDir)/runtime_stderr.log"
$runtimePID = $procRuntime.Id
Write-Log "[8/22] Started Go Runtime (PID: $runtimePID)"

# Wait a moment for startup
Start-Sleep -Seconds 2

# ---- 9. Record Runtime PID ----
Write-Log "[9/22] Runtime PID: $runtimePID"

# ---- 10. Wait for Go Runtime ready ----
$healthUrl = "http://127.0.0.1:$RuntimePort/v1/health"
$ready = $false
$timeout = [datetime]::Now.AddSeconds($ReadyTimeoutSec)
while ([datetime]::Now -lt $timeout) {
    try {
        $resp = Invoke-RestMethod -Uri $healthUrl -Method Get -ErrorAction Stop
        $healthStatus = if ($resp.data) { $resp.data.status } else { $resp.status }
        $healthData = if ($resp.data) { $resp.data } else { $resp }
        $audiocppAlive = $healthData.audiocpp_alive
        $audiocppState = $healthData.audiocpp_state
        $audiocppPid = $healthData.audiocpp_pid
        if ($healthStatus -eq "ok" -and $audiocppAlive -eq $true -and $audiocppState -eq "running" -and $audiocppPid -gt 0) {
            $ready = $true
            Write-Log "[10/22] Go Runtime ready (status=ok, audiocpp_alive=$audiocppAlive, state=$audiocppState, pid=$audiocppPid)"
            break
        }
    } catch {
        # Not ready yet
    }
    Start-Sleep -Milliseconds 500
}
if (-not $ready) {
    Write-Host "REAL_SMOKE_FAIL: Go Runtime did not become ready within ${ReadyTimeoutSec}s"
    Stop-Process -Id $runtimePID -Force -ErrorAction SilentlyContinue
    exit 1
}

# ---- 11. Get audiocpp child PID ----
$childPID = 0
$childState = ""
try {
    $healthData = Invoke-RestMethod -Uri $healthUrl -Method Get -ErrorAction Stop
    $data = if ($healthData.data) { $healthData.data } else { $healthData }
    $childPID = $data.audiocpp_pid
    $childState = $data.audiocpp_state
    Write-Log "[11/22] audiocpp child PID: $childPID, state: $childState"
} catch {
    Write-Host "REAL_SMOKE_FAIL: Could not read health endpoint for child PID"
    Stop-Process -Id $runtimePID -Force -ErrorAction SilentlyContinue
    exit 1
}

# ---- 12. Send transcription request ----
$transcribeUrl = "http://127.0.0.1:$RuntimePort/v1/audio/transcriptions"
Write-Log "[12/22] Sending POST $transcribeUrl"

# Save request metadata
$wavSha256 = ""
try { $wavSha256 = (Get-FileHash -Path $wavPath -Algorithm SHA256 -ErrorAction Stop).Hash } catch {}
$requestInfo = @{
    url = $transcribeUrl
    method = "POST"
    wav_path = $wavPath
    wav_sha256 = $wavSha256
    timestamp = (Get-Date -Format 'o')
} | ConvertTo-Json
Write-FileUtf8NoBom -path (Join-Path $ArtifactsDir "request.json") -content $requestInfo

$responsePath = Join-Path $ArtifactsDir "response.json"
$transcriptionText = ""
$httpStatus = 0
$responseReceived = $false
$responseParsed = $false
$inferenceStart = Get-Date

try {
    # Use .NET HttpClient for multipart upload (PowerShell 5.1 compatible)
    Add-Type -AssemblyName System.Net.Http
    $client = New-Object System.Net.Http.HttpClient
    $content = New-Object System.Net.Http.MultipartFormDataContent
    $fileStream = [System.IO.File]::OpenRead($wavPath)
    $fileContent = New-Object System.Net.Http.StreamContent($fileStream)
    $fileContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse("audio/wav")
    $content.Add($fileContent, "file", [System.IO.Path]::GetFileName($wavPath))
    # Add model field (required by API)
    $modelContent = New-Object System.Net.Http.StringContent("citrinet-asr")
    $content.Add($modelContent, "model")
    
    $response = $client.PostAsync($transcribeUrl, $content).Result
    $responseReceived = $true
    $inferenceEnd = Get-Date
    $inferenceMs = [math]::Round(($inferenceEnd - $inferenceStart).TotalMilliseconds)
    $httpStatus = [int]$response.StatusCode
    
    if ($response.IsSuccessStatusCode) {
        $responseBody = $response.Content.ReadAsStringAsync().Result
        $responseJson = $responseBody | ConvertFrom-Json
        $responseParsed = $true
        $responseJsonStr = $responseJson | ConvertTo-Json -Depth 10
        Write-FileUtf8NoBom -path $responsePath -content $responseJsonStr
        Write-Log "[13/22] Response saved to $responsePath"
        
        # Parse transcription text
        if ($responseJson.text) {
            $transcriptionText = $responseJson.text
        } elseif ($responseJson.data -and $responseJson.data.text) {
            $transcriptionText = $responseJson.data.text
        } else {
            $transcriptionText = $responseBody
        }
        Write-Log "[12/22] HTTP 200, transcription received"
    } else {
        $errorBody = $response.Content.ReadAsStringAsync().Result
        Write-FileUtf8NoBom -path $responsePath -content $errorBody
        Write-Log "[12/22] HTTP $httpStatus - transcription failed: $errorBody"
    }
    
    $fileStream.Close()
    $client.Dispose()
} catch {
    $inferenceEnd = Get-Date
    $inferenceMs = [math]::Round(($inferenceEnd - $inferenceStart).TotalMilliseconds)
    Write-Log "[12/22] Exception during transcription: $_"
}

# ---- 14. Parse transcription text ----
Write-Log "[14/22] Transcription text: '$transcriptionText'"

# ---- 15. Verify non-empty ----
$textNonEmpty = (-not [string]::IsNullOrWhiteSpace($transcriptionText))
if ($textNonEmpty) {
    Write-Log "[15/22] PASS - transcription non-empty"
} else {
    Write-Log "[15/22] FAIL - transcription empty"
}

# ---- 16. Loose match ----
$matchResult = "N/A"
$matchPassed = $false
if ($textNonEmpty -and $ExpectedText) {
    # Case-insensitive loose matching: check if at least 2 expected words appear
    $expectedWords = $ExpectedText -split '\s+'
    $transWords = $transcriptionText.ToLower() -split '\s+'
    $matchedWords = $expectedWords | Where-Object { $transWords -contains $_.ToLower() } | Measure-Object | Select-Object -ExpandProperty Count
    $matchPercent = [math]::Round(($matchedWords / $expectedWords.Count) * 100, 1)
    $matchResult = "$matchedWords/$($expectedWords.Count) words matched ($matchPercent%)"
    Write-Log "[16/22] Expected: '$ExpectedText'"
    Write-Log "[16/22] Loose match: $matchResult"
    $matchPassed = ($matchedWords -ge 5 -and $matchPercent -ge 50)
    if ($matchPassed) {
        Write-Log "[16/22] PASS - sufficient word match ($matchedWords words, $matchPercent%)"
    } else {
        Write-Log "[16/22] FAIL - insufficient word match ($matchedWords words, $matchPercent%), need >=5 words and >=50%"
    }
}

# ---- 17. Graceful shutdown ----
$shutdownStart = Get-Date
$gracefulShutdown = $false
$forceKillUsed = $false

# Try graceful shutdown via API
try {
    $shutdownResp = Invoke-RestMethod -Uri "http://127.0.0.1:$RuntimePort/v1/shutdown" -Method Post -TimeoutSec 5 -ErrorAction Stop
    Write-Log "[17/22] Shutdown API responded"
    $gracefulShutdown = $true
} catch {
    Write-Log "[17/22] Shutdown API failed: $_"
}

# Wait up to 5 seconds for graceful exit
if (-not (Wait-PidExit -procId $runtimePID -timeoutSeconds 5)) {
    Write-Log "[17/22] Runtime still alive after graceful shutdown, using taskkill /T /F"
    taskkill /PID $runtimePID /T /F 2>&1 | Out-Null
    $forceKillUsed = $true
}

$shutdownEnd = Get-Date
$shutdownMs = [math]::Round(($shutdownEnd - $shutdownStart).TotalMilliseconds)
Write-Log "[17/22] Shutdown completed in ${shutdownMs}ms (graceful=$gracefulShutdown, forceKill=$forceKillUsed)"

# ---- 18. Wait for Runtime PID to exit ----
$runtimeExited = Wait-PidExit -procId $runtimePID -timeoutSeconds 10
if ($runtimeExited) {
    Write-Log "[18/22] Runtime PID $runtimePID exited"
} else {
    Write-Log "[18/22] Runtime PID $runtimePID still alive after timeout"
}

# ---- 19. Wait for child PID to exit (taskkill /T should have cleaned it) ----
$childExited = $true
$childAliveAfter = $false
if ($childPID -gt 0) {
    $childExited = Wait-PidExit -procId $childPID -timeoutSeconds 5
    if ($childExited) {
        Write-Log "[19/22] Child PID $childPID exited"
    } else {
        Write-Log "[19/22] Child PID $childPID still alive, force killing"
        taskkill /PID $childPID /F 2>&1 | Out-Null
        Start-Sleep 1
        $childExited = -not (Get-Process -Id $childPID -ErrorAction SilentlyContinue)
        $childAliveAfter = -not $childExited
        Write-Log "[19/22] Child force kill result: exited=$childExited"
    }
}

# ---- 20. Check port release (retry for TIME_WAIT) ----
$runtimePortFree = $false
$audioCppPortFree = $false
for ($retry = 0; $retry -lt 15; $retry++) {
    $runtimePortFree = Test-PortFree -port $RuntimePort
    $audioCppPortFree = Test-PortFree -port $AudioCppPort
    if ($runtimePortFree -and $audioCppPortFree) { break }
    Start-Sleep -Seconds 2
}
Write-Log "[20/22] Runtime port $RuntimePort free: $runtimePortFree"
Write-Log "[20/22] AudioCpp port $AudioCppPort free: $audioCppPortFree"

# Save port state
$portsAfter = @{
    runtime_port = $RuntimePort
    runtime_port_free = $runtimePortFree
    audiocpp_port = $AudioCppPort
    audiocpp_port_free = $audioCppPortFree
    checked_at = (Get-Date -Format 'o')
} | ConvertTo-Json
Write-FileUtf8NoBom -path (Join-Path $ArtifactsDir "ports-after.json") -content $portsAfter

# ---- Capture process status ----
$procSnapshot = @{
    runtime = @{
        pid = $runtimePID
        alive = (Get-Process -Id $runtimePID -ErrorAction SilentlyContinue) -ne $null
    }
    audiocpp = @{
        pid = $childPID
        alive = (Get-Process -Id $childPID -ErrorAction SilentlyContinue) -ne $null
    }
} | ConvertTo-Json
Write-FileUtf8NoBom -path (Join-Path $ArtifactsDir "process-status.json") -content $procSnapshot
Write-Log "[proc] Process status saved"

# ---- Generate result.md ----
$endTime = Get-Date
$totalMs = [math]::Round(($endTime - $startTime).TotalMilliseconds)
$gitCommit = (git rev-parse HEAD 2>$null)
if (-not $gitCommit) {
    Write-Host "REAL_SMOKE_FAIL: Could not determine git commit"
    exit 1
}
# ---- Get audio.cpp upstream SHA ----
$upstreamSha = "unknown"
try {
    $upstreamSha = git -C (Join-Path $repoRoot "audio.cpp") rev-parse HEAD 2>$null
    if (-not $upstreamSha -or $upstreamSha -eq "unknown") {
        throw "Could not get audio.cpp HEAD"
    }
    Write-Log "[upstream] audio.cpp SHA: $upstreamSha"
} catch {
    Write-Host "REAL_SMOKE_FAIL: Could not get audio.cpp upstream SHA: $_"
    exit 1
}
$wavSha = "unknown"
try { $wavSha = (Get-FileHash -Path $wavPath -Algorithm SHA256 -ErrorAction Stop).Hash } catch {} 

$modelSha = @{}
foreach ($mf in $requiredModels) {
    $mp = Join-Path $modelPath $mf
    if (Test-Path $mp) {
        try { $modelSha[$mf] = (Get-FileHash -Path $mp -Algorithm SHA256 -ErrorAction Stop).Hash } catch { $modelSha[$mf] = "unknown" }
    }
}
$modelShaJson = $modelSha | ConvertTo-Json -Compress

$finalPass = ($httpStatus -eq 200 -and $textNonEmpty -and $matchPassed -and ($childPID -gt 0) -and ($childState -eq "running") -and $runtimeExited -and $childExited -and $runtimePortFree -and $audioCppPortFree)

$result = @"
# Real ASR Smoke Test Result

| Field | Value |
|-------|-------|
| Execution Time | $(Get-Date $startTime -Format 'yyyy-MM-dd HH:mm:ss') |
| Total Duration | ${totalMs}ms |
| Git Commit | $gitCommit |
| Go Runtime Binary | $runtimePath |
| audiocpp_server Binary | $serverPath |
| audio.cpp Upstream SHA | $upstreamSha |
| Citrinet Model SHA256 | $modelShaJson |
| Input WAV SHA256 | $wavSha |
| Runtime PID | $runtimePID |
| Child PID | $childPID |
| Request URL | POST $transcribeUrl |
| HTTP Status | $httpStatus |
| Transcription | $transcriptionText |
| Expected Transcription | $ExpectedText |
| Match Result | $matchResult |
| Inference Duration | ${inferenceMs}ms |
| Shutdown Duration | ${shutdownMs}ms |
| Graceful Shutdown | $gracefulShutdown |
| Force Kill Used | $forceKillUsed |
| Runtime Exited Cleanly | $runtimeExited |
| Child Exited Cleanly | $childExited |
| Response Received | $responseReceived |
| Response Parsed | $responseParsed |
| Runtime Port Free | $runtimePortFree |
| AudioCpp Port Free | $audioCppPortFree |

## Verdict

**$(if ($finalPass) { 'REAL_SMOKE_PASS' } else { 'REAL_SMOKE_FAIL' })**
"@
Write-FileUtf8NoBom -path (Join-Path $ArtifactsDir "result.md") -content $result
Write-Log "[result] Written to $(Join-Path $ArtifactsDir 'result.md')"

Write-Log "=== REAL SMOKE TEST $(if ($finalPass) { 'PASS' } else { 'FAIL' }) ==="

if ($finalPass) {
    Write-Host "REAL_SMOKE_PASS"
    exit 0
} else {
    Write-Host "REAL_SMOKE_FAIL"
    exit 1
}
