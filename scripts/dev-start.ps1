<#
.SYNOPSIS
    DiscoPanel Dual-Mode Rapid Development Launcher (PowerShell)
.DESCRIPTION
    Launches Go backend and Svelte 5 frontend in parallel with live hot-reloading.
    Provisions runtime directories, seeds test data, and manages process lifecycles.
#>

[CmdletBinding()]
param (
    [switch]$Clean,
    [switch]$NoAir,
    [int]$BackendPort = 8080,
    [int]$FrontendPort = 5173
)

$ErrorActionPreference = 'Stop'

# ── Colors & Banner ───────────────────────────────────────────────────────────
$ESC = [char]27
$C_RESET   = "$ESC[0m"
$C_BOLD    = "$ESC[1m"
$C_GREEN   = "$ESC[32m"
$C_YELLOW  = "$ESC[33m"
$C_RED     = "$ESC[31m"
$C_CYAN    = "$ESC[36m"
$C_MAGENTA = "$ESC[35m"

# Resolve repo root directory (one level up from scripts/)
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot  = Split-Path -Parent $ScriptDir
Set-Location $RepoRoot

Write-Host ""
Write-Host "$C_BOLD$C_MAGENTA  ============================================================$C_RESET"
Write-Host "$C_BOLD$C_CYAN     DISCOPANEL RAPID LOCAL DEVELOPMENT ENVIRONMENT           $C_RESET"
Write-Host "$C_BOLD$C_MAGENTA  ============================================================$C_RESET"
Write-Host ""

# ── Clean Option ──────────────────────────────────────────────────────────────
if ($Clean) {
    Write-Host "$C_YELLOW[*] Cleaning dev data directories...$C_RESET"
    if (Test-Path "$RepoRoot/data") {
        Remove-Item -Path "$RepoRoot/data" -Recurse -Force -ErrorAction SilentlyContinue
    }
    Write-Host "$C_GREEN[+] Dev data cleared.$C_RESET"
}

# ── Ensure Runtime Data Directories ──────────────────────────────────────────
$RequiredDirs = @(
    "$RepoRoot/data",
    "$RepoRoot/data/servers",
    "$RepoRoot/data/backups",
    "$RepoRoot/data/temp",
    "$RepoRoot/data/modules",
    "$RepoRoot/data/logs",
    "$RepoRoot/dev"
)

foreach ($dir in $RequiredDirs) {
    if (-not (Test-Path $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
}

# ── Seed Database If Available ───────────────────────────────────────────────
$SeededDb = "$RepoRoot/dev/discopanel.db"
$LiveDb   = "$RepoRoot/data/discopanel.db"
if ((Test-Path $SeededDb) -and (-not (Test-Path $LiveDb))) {
    Write-Host "$C_CYAN[*] Restoring seeded development database from dev/discopanel.db...$C_RESET"
    Copy-Item $SeededDb $LiveDb -Force
}

# ── Set Default Dev Environment Variables ─────────────────────────────────────
if (-not $env:DISCOPANEL_SERVER_PORT) { $env:DISCOPANEL_SERVER_PORT = "$BackendPort" }
if (-not $env:DISCOPANEL_DATABASE_PATH) { $env:DISCOPANEL_DATABASE_PATH = "./data/discopanel.db" }
if (-not $env:DISCOPANEL_STORAGE_DATA_DIR) { $env:DISCOPANEL_STORAGE_DATA_DIR = "./data" }
if (-not $env:DISCOPANEL_STORAGE_BACKUP_DIR) { $env:DISCOPANEL_STORAGE_BACKUP_DIR = "./data/backups" }
if (-not $env:DISCOPANEL_STORAGE_TEMP_DIR) { $env:DISCOPANEL_STORAGE_TEMP_DIR = "./data/temp" }
if (-not $env:DISCOPANEL_LOGGING_FILE_PATH) { $env:DISCOPANEL_LOGGING_FILE_PATH = "./data/logs/discopanel.log" }
if (-not $env:DISCOPANEL_AUTH_LOCAL_ALLOW_REGISTRATION) { $env:DISCOPANEL_AUTH_LOCAL_ALLOW_REGISTRATION = "true" }

# ── Check & Install Frontend Dependencies ─────────────────────────────────────
$FrontendDir = "$RepoRoot/web/discopanel"
$NodeModules = "$FrontendDir/node_modules"
$npmExe = (Get-Command "npm.cmd" -ErrorAction SilentlyContinue).Source
if (-not $npmExe) {
    $npmExe = (Get-Command "npm" -ErrorAction SilentlyContinue).Source
}

if (-not (Test-Path $NodeModules)) {
    Write-Host "$C_YELLOW[*] Installing frontend node_modules (first run)...$C_RESET"
    if ($npmExe) {
        Push-Location $FrontendDir
        try {
            & $npmExe install
        } finally {
            Pop-Location
        }
    } else {
        Write-Warning "npm not found! Frontend may fail to start."
    }
}

# ── Detect Air vs Standard Go Run ─────────────────────────────────────────────
$airCmd = Get-Command "air" -ErrorAction SilentlyContinue
$useAir = ($null -ne $airCmd) -and (-not $NoAir)

Write-Host "  $C_BOLD$C_GREEN-> Frontend Dev Server:$C_RESET http://localhost:$FrontendPort"
Write-Host "  $C_BOLD$C_GREEN-> Backend ConnectRPC :$C_RESET http://localhost:$BackendPort"
Write-Host "  $C_BOLD$C_GREEN-> Minecraft L4 Proxy :$C_RESET localhost:25565"
if ($useAir) {
    Write-Host "  $C_BOLD$C_CYAN-> Backend Live Reload:$C_RESET Enabled (via Air)"
} else {
    Write-Host "  $C_BOLD$C_CYAN-> Backend Live Reload:$C_RESET Disabled (using 'go run cmd/discopanel/main.go')"
}
Write-Host ""
Write-Host "  $C_GRAY Press Ctrl+C to terminate all development services.$C_RESET"
Write-Host ""

# ── Start Frontend & Backend Processes ────────────────────────────────────────
$pidsToClean = [System.Collections.Generic.List[int]]::new()

# Start Frontend Job
Write-Host "$C_CYAN[+] Starting Svelte 5 / Vite Frontend...$C_RESET"
$frontendProc = Start-Process -FilePath ($npmExe ?? "npm") -ArgumentList "run dev" `
    -WorkingDirectory $FrontendDir -PassThru -NoNewWindow
$pidsToClean.Add($frontendProc.Id)

# Small delay to let Vite initialize port
Start-Sleep -Milliseconds 800

# Start Backend Process
Write-Host "$C_CYAN[+] Starting Go Backend Daemon...$C_RESET"
$backendProc = $null
if ($useAir) {
    $backendProc = Start-Process -FilePath "air" -WorkingDirectory $RepoRoot -PassThru -NoNewWindow
} else {
    $backendProc = Start-Process -FilePath "go" -ArgumentList "run cmd/discopanel/main.go" `
        -WorkingDirectory $RepoRoot -PassThru -NoNewWindow
}
$pidsToClean.Add($backendProc.Id)

# ── Monitor Processes & Graceful Shutdown ─────────────────────────────────────
function Stop-DevProcesses {
    Write-Host ""
    Write-Host "$C_YELLOW[*] Shutting down DiscoPanel development processes...$C_RESET"
    foreach ($pId in $pidsToClean) {
        try {
            $proc = Get-Process -Id $pId -ErrorAction SilentlyContinue
            if ($proc) {
                # Stop process tree on Windows
                taskkill /PID $pId /T /F 2>&1 | Out-Null
            }
        } catch {}
    }
    Write-Host "$C_GREEN[+] All development processes stopped.$C_RESET"
}

try {
    while (-not $frontendProc.HasExited -and -not $backendProc.HasExited) {
        Start-Sleep -Seconds 1
    }
} catch {
    # Catches Ctrl+C or interruption
} finally {
    Stop-DevProcesses
}
