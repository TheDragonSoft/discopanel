<#
.SYNOPSIS
    DiscoPanel Rapid Development Pre-Flight Environment Checker (PowerShell)
.DESCRIPTION
    Verifies Go 1.24+, Node.js 20+, npm/pnpm, Docker Daemon, Buf CLI, Delve (dlv),
    and local development port availability.
#>

[CmdletBinding()]
param (
    [switch]$VerboseOutput
)

$ErrorActionPreference = 'Continue'

# ── ANSI Color Helpers ────────────────────────────────────────────────────────
$ESC = [char]27
$C_RESET   = "$ESC[0m"
$C_BOLD    = "$ESC[1m"
$C_GREEN   = "$ESC[32m"
$C_YELLOW  = "$ESC[33m"
$C_RED     = "$ESC[31m"
$C_CYAN    = "$ESC[36m"
$C_GRAY    = "$ESC[90m"

function Write-Header {
    Write-Host ""
    Write-Host "$C_BOLD$C_CYAN============================================================$C_RESET"
    Write-Host "$C_BOLD$C_CYAN   DiscoPanel Developer Environment Pre-Flight Checker       $C_RESET"
    Write-Host "$C_BOLD$C_CYAN============================================================$C_RESET"
    Write-Host ""
}

$AllPassed = $true
$WarningCount = 0

function Report-Check {
    param(
        [string]$Name,
        [string]$Status, # "PASS", "WARN", "FAIL"
        [string]$Details,
        [string]$FixAdvice = ""
    )
    switch ($Status) {
        "PASS" {
            Write-Host "  [$C_GREEN PASS $C_RESET] $C_BOLD$Name$C_RESET : $Details"
        }
        "WARN" {
            $script:WarningCount++
            Write-Host "  [$C_YELLOW WARN $C_RESET] $C_BOLD$Name$C_RESET : $Details"
            if ($FixAdvice) {
                Write-Host "         $C_YELLOW Advice: $FixAdvice$C_RESET"
            }
        }
        "FAIL" {
            $script:AllPassed = $false
            Write-Host "  [$C_RED FAIL $C_RESET] $C_BOLD$Name$C_RESET : $Details"
            if ($FixAdvice) {
                Write-Host "         $C_RED Fix: $FixAdvice$C_RESET"
            }
        }
    }
}

Write-Header

# ── 1. Check Go Version ───────────────────────────────────────────────────────
$goCmd = Get-Command "go" -ErrorAction SilentlyContinue
if ($null -eq $goCmd) {
    Report-Check -Name "Go Toolchain" -Status "FAIL" `
        -Details "Go not found in PATH." `
        -FixAdvice "Install Go 1.24+ from https://go.dev/dl/ and add it to PATH."
} else {
    try {
        $goVerRaw = (& go version) 2>&1
        if ($goVerRaw -match 'go(\d+)\.(\d+)(\.(\d+))?') {
            $major = [int]$matches[1]
            $minor = [int]$matches[2]
            if ($major -gt 1 -or ($major -eq 1 -and $minor -ge 24)) {
                Report-Check -Name "Go Toolchain" -Status "PASS" -Details "$goVerRaw (Go 1.24+ ready)"
            } else {
                Report-Check -Name "Go Toolchain" -Status "WARN" `
                    -Details "$goVerRaw (DiscoPanel requires Go 1.24+ for full compatibility)" `
                    -FixAdvice "Upgrade Go to 1.24+ at https://go.dev/dl/"
            }
        } else {
            Report-Check -Name "Go Toolchain" -Status "PASS" -Details "$goVerRaw"
        }
    } catch {
        Report-Check -Name "Go Toolchain" -Status "FAIL" -Details "Failed executing 'go version'." -FixAdvice "$_"
    }
}

# ── 2. Check Node.js Version ──────────────────────────────────────────────────
$nodeCmd = Get-Command "node" -ErrorAction SilentlyContinue
if ($null -eq $nodeCmd) {
    Report-Check -Name "Node.js Runtime" -Status "FAIL" `
        -Details "Node.js not found in PATH." `
        -FixAdvice "Install Node.js 20+ LTS from https://nodejs.org/"
} else {
    try {
        $nodeVerRaw = (& node --version) 2>&1
        if ($nodeVerRaw -match 'v(\d+)\.') {
            $nodeMajor = [int]$matches[1]
            if ($nodeMajor -ge 20) {
                Report-Check -Name "Node.js Runtime" -Status "PASS" -Details "$nodeVerRaw (v20+ ready for Svelte 5 / Vite 7)"
            } else {
                Report-Check -Name "Node.js Runtime" -Status "WARN" `
                    -Details "$nodeVerRaw (Node.js 20+ recommended for Vite 7)" `
                    -FixAdvice "Upgrade Node.js to >= 20.x from https://nodejs.org/"
            }
        } else {
            Report-Check -Name "Node.js Runtime" -Status "PASS" -Details "$nodeVerRaw"
        }
    } catch {
        Report-Check -Name "Node.js Runtime" -Status "FAIL" -Details "Failed executing 'node --version'." -FixAdvice "$_"
    }
}

# ── 3. Check Package Manager (npm / pnpm) ──────────────────────────────────────
$npmCmd = Get-Command "npm.cmd" -ErrorAction SilentlyContinue
if ($null -eq $npmCmd) {
    $npmCmd = Get-Command "npm" -ErrorAction SilentlyContinue
}

if ($null -eq $npmCmd) {
    Report-Check -Name "Node Package Manager" -Status "FAIL" `
        -Details "npm not found." `
        -FixAdvice "Reinstall Node.js or ensure npm is on PATH."
} else {
    try {
        $npmVer = (& $npmCmd.Source --version) 2>&1
        Report-Check -Name "Node Package Manager" -Status "PASS" -Details "npm v$npmVer"
    } catch {
        Report-Check -Name "Node Package Manager" -Status "WARN" -Details "npm found at $($npmCmd.Source) but failed executing."
    }
}

# ── 4. Check Docker Daemon ────────────────────────────────────────────────────
$dockerCmd = Get-Command "docker" -ErrorAction SilentlyContinue
if ($null -eq $dockerCmd) {
    Report-Check -Name "Docker Engine" -Status "WARN" `
        -Details "Docker CLI not found in PATH." `
        -FixAdvice "Install Docker Desktop for Windows (with WSL2 engine) from https://www.docker.com/"
} else {
    try {
        $dockerVer = (& docker --version) 2>&1
        # Test daemon responsiveness
        $dockerPing = (& docker info --format '{{.ServerVersion}}') 2>&1
        if ($LASTEXITCODE -eq 0) {
            Report-Check -Name "Docker Engine" -Status "PASS" -Details "$dockerVer (Daemon active, Server v$dockerPing)"
        } else {
            Report-Check -Name "Docker Engine" -Status "WARN" `
                -Details "$dockerVer found, but Docker daemon is NOT running or accessible." `
                -FixAdvice "Start Docker Desktop. Ensure 'Expose daemon on tcp://localhost:2375 without TLS' or named pipes are active."
        }
    } catch {
        Report-Check -Name "Docker Engine" -Status "WARN" -Details "Error querying Docker CLI." -FixAdvice "$_"
    }
}

# ── 5. Check Buf CLI (Protobuf Compiler) ──────────────────────────────────────
$bufCmd = Get-Command "buf" -ErrorAction SilentlyContinue
if ($null -ne $bufCmd) {
    try {
        $bufVer = (& buf --version) 2>&1
        Report-Check -Name "Buf CLI (Native)" -Status "PASS" -Details "buf version $bufVer"
    } catch {
        Report-Check -Name "Buf CLI (Native)" -Status "PASS" -Details "buf found"
    }
} else {
    # Check if Docker is available to run buf fallback
    if ($null -ne $dockerCmd) {
        Report-Check -Name "Buf CLI (Protobuf)" -Status "PASS" `
            -Details "Native 'buf' not installed; Docker fallback (bufbuild/buf:latest) available via 'make gen'."
    } else {
        Report-Check -Name "Buf CLI (Protobuf)" -Status "WARN" `
            -Details "Neither native 'buf' CLI nor Docker available for proto compilation." `
            -FixAdvice "Install Buf CLI via: 'go install github.com/bufbuild/buf/cmd/buf@latest' or 'winget install bufbuild.buf'"
    }
}

# ── 6. Check Delve Debugger (dlv) ─────────────────────────────────────────────
$dlvCmd = Get-Command "dlv" -ErrorAction SilentlyContinue
if ($null -ne $dlvCmd) {
    Report-Check -Name "Delve Debugger (dlv)" -Status "PASS" -Details "Installed (VS Code Go debugging ready)"
} else {
    Report-Check -Name "Delve Debugger (dlv)" -Status "WARN" `
        -Details "dlv not found on PATH (needed for F5 Go debugging)." `
        -FixAdvice "Install Delve via: 'go install github.com/go-delve/delve/cmd/dlv@latest'"
}

# ── 7. Check Hot Reload Utility (Air) ─────────────────────────────────────────
$airCmd = Get-Command "air" -ErrorAction SilentlyContinue
if ($null -ne $airCmd) {
    Report-Check -Name "Go Live Reload (Air)" -Status "PASS" -Details "Air installed (instant backend reload enabled)"
} else {
    Report-Check -Name "Go Live Reload (Air)" -Status "WARN" `
        -Details "Air not installed (optional, enables automatic Go recompilation on save)." `
        -FixAdvice "Install Air via: 'go install github.com/air-verse/air@latest'"
}

# ── 8. Check Port Availability ────────────────────────────────────────────────
$portsToCheck = @(
    @{ Port = 8080;  Name = "Backend / ConnectRPC API" },
    @{ Port = 5173;  Name = "Frontend Vite Dev Server" },
    @{ Port = 25565; Name = "Minecraft Reverse Proxy" }
)

Write-Host ""
Write-Host "  $C_BOLD$C_CYAN Checking Port Conflicts (8080, 5173, 25565):$C_RESET"

foreach ($p in $portsToCheck) {
    $port = $p.Port
    $name = $p.Name
    $occupied = $false
    try {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $port)
        $listener.Start()
        $listener.Stop()
        Report-Check -Name "Port $port ($name)" -Status "PASS" -Details "Available"
    } catch {
        Report-Check -Name "Port $port ($name)" -Status "WARN" `
            -Details "Port $port is currently in use by another process." `
            -FixAdvice "Check active processes with 'Get-NetTCPConnection -LocalPort $port' or configure a different port in .env."
    }
}

# ── Summary & Recommendation ──────────────────────────────────────────────────
Write-Host ""
Write-Host "$C_BOLD$C_CYAN============================================================$C_RESET"
if ($AllPassed -and $WarningCount -eq 0) {
    Write-Host "$C_BOLD$C_GREEN [PERFECT] All development requirements and tooling are ready! $C_RESET"
    Write-Host " Start development now with: $C_BOLD./scripts/dev-start.ps1$C_RESET or in VS Code press $C_BOLD Ctrl+Shift+B $C_RESET"
} elseif ($AllPassed) {
    Write-Host "$C_BOLD$C_YELLOW [READY WITH WARNINGS] Minimum requirements satisfied ($WarningCount warnings). $C_RESET"
    Write-Host " You can start development with: $C_BOLD./scripts/dev-start.ps1$C_RESET"
} else {
    Write-Host "$C_BOLD$C_RED [ACTION REQUIRED] Some essential tools are missing. Please review the items above. $C_RESET"
}
Write-Host "$C_BOLD$C_CYAN============================================================$C_RESET"
Write-Host ""
