#!/usr/bin/env bash
# ==============================================================================
# DiscoPanel Rapid Development Pre-Flight Environment Checker (Bash)
# ==============================================================================
# Verifies Go 1.24+, Node.js 20+, npm/pnpm, Docker Daemon, Buf CLI, Delve, and ports.
# ==============================================================================

set -u

# Colors
C_RESET='\033[0m'
C_BOLD='\033[1m'
C_GREEN='\033[32m'
C_YELLOW='\033[33m'
C_RED='\033[31m'
C_CYAN='\033[36m'

ALL_PASSED=1
WARN_COUNT=0

report_pass() {
    echo -e "  [${C_GREEN} PASS ${C_RESET}] ${C_BOLD}$1${C_RESET} : $2"
}

report_warn() {
    WARN_COUNT=$((WARN_COUNT + 1))
    echo -e "  [${C_YELLOW} WARN ${C_RESET}] ${C_BOLD}$1${C_RESET} : $2"
    if [ -n "${3:-}" ]; then
        echo -e "         ${C_YELLOW}Advice: $3${C_RESET}"
    fi
}

report_fail() {
    ALL_PASSED=0
    echo -e "  [${C_RED} FAIL ${C_RESET}] ${C_BOLD}$1${C_RESET} : $2"
    if [ -n "${3:-}" ]; then
        echo -e "         ${C_RED}Fix: $3${C_RESET}"
    fi
}

echo ""
echo -e "${C_BOLD}${C_CYAN}============================================================${C_RESET}"
echo -e "${C_BOLD}${C_CYAN}   DiscoPanel Developer Environment Pre-Flight Checker       ${C_RESET}"
echo -e "${C_BOLD}${C_CYAN}============================================================${C_RESET}"
echo ""

# 1. Check Go
if command -v go >/dev/null 2>&1; then
    GO_VER_RAW=$(go version)
    GO_VER=$(echo "$GO_VER_RAW" | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
    GO_MAJOR=$(echo "$GO_VER" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VER" | cut -d. -f2)

    if [ "$GO_MAJOR" -gt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -ge 24 ]; }; then
        report_pass "Go Toolchain" "$GO_VER_RAW (Go 1.24+ ready)"
    else
        report_warn "Go Toolchain" "$GO_VER_RAW (Go 1.24+ recommended)" "Upgrade Go from https://go.dev/dl/"
    fi
else
    report_fail "Go Toolchain" "Go is not installed or not found on PATH." "Install Go 1.24+ from https://go.dev/dl/"
fi

# 2. Check Node.js
if command -v node >/dev/null 2>&1; then
    NODE_VER_RAW=$(node --version)
    NODE_MAJOR=$(echo "$NODE_VER_RAW" | tr -d 'v' | cut -d. -f1)

    if [ "$NODE_MAJOR" -ge 20 ]; then
        report_pass "Node.js Runtime" "$NODE_VER_RAW (v20+ ready for Svelte 5 / Vite 7)"
    else
        report_warn "Node.js Runtime" "$NODE_VER_RAW (Node.js 20+ recommended for Vite 7)" "Upgrade Node from https://nodejs.org/"
    fi
else
    report_fail "Node.js Runtime" "Node.js not installed or not on PATH." "Install Node.js 20+ from https://nodejs.org/"
fi

# 3. Check npm / pnpm
if command -v npm >/dev/null 2>&1; then
    NPM_VER=$(npm --version)
    report_pass "Package Manager" "npm v$NPM_VER"
else
    report_fail "Package Manager" "npm not found." "Install npm with Node.js."
fi

# 4. Check Docker
if command -v docker >/dev/null 2>&1; then
    DOCKER_VER=$(docker --version)
    if docker info >/dev/null 2>&1; then
        report_pass "Docker Engine" "$DOCKER_VER (Daemon active and responsive)"
    else
        report_warn "Docker Engine" "$DOCKER_VER found, but Docker daemon is NOT running." "Start Docker Desktop or service docker start."
    fi
else
    report_warn "Docker Engine" "Docker CLI not found." "Install Docker from https://www.docker.com/"
fi

# 5. Check Buf
if command -v buf >/dev/null 2>&1; then
    BUF_VER=$(buf --version 2>&1)
    report_pass "Buf CLI (Native)" "buf version $BUF_VER"
elif command -v docker >/dev/null 2>&1; then
    report_pass "Buf CLI (Protobuf)" "Docker fallback (bufbuild/buf:latest) available via 'make gen'."
else
    report_warn "Buf CLI (Protobuf)" "Neither native 'buf' nor Docker found." "Install Buf: 'go install github.com/bufbuild/buf/cmd/buf@latest'"
fi

# 6. Check Delve
if command -v dlv >/dev/null 2>&1; then
    report_pass "Delve Debugger (dlv)" "Installed (VS Code Go debugging ready)"
else
    report_warn "Delve Debugger (dlv)" "dlv not found on PATH." "Install Delve: 'go install github.com/go-delve/delve/cmd/dlv@latest'"
fi

# 7. Check Air
if command -v air >/dev/null 2>&1; then
    report_pass "Go Live Reload (Air)" "Air installed (instant backend reload enabled)"
else
    report_warn "Go Live Reload (Air)" "Air not installed (optional)." "Install Air: 'go install github.com/air-verse/air@latest'"
fi

# 8. Check Ports
echo ""
echo -e "  ${C_BOLD}${C_CYAN}Checking Port Conflicts (8080, 5173, 25565):${C_RESET}"
for PORT in 8080 5173 25565; do
    if command -v nc >/dev/null 2>&1; then
        if nc -z 127.0.0.1 "$PORT" >/dev/null 2>&1; then
            report_warn "Port $PORT" "Port $PORT is currently in use." "Check active processes with lsof -i :$PORT or netstat."
        else
            report_pass "Port $PORT" "Available"
        fi
    elif command -v lsof >/dev/null 2>&1; then
        if lsof -Pi :"$PORT" -sTCP:LISTEN -t >/dev/null 2>&1; then
            report_warn "Port $PORT" "Port $PORT is currently in use." "Check active processes with lsof -i :$PORT"
        else
            report_pass "Port $PORT" "Available"
        fi
    else
        report_pass "Port $PORT" "Port check skipped (nc/lsof not installed)"
    fi
done

echo ""
echo -e "${C_BOLD}${C_CYAN}============================================================${C_RESET}"
if [ "$ALL_PASSED" -eq 1 ] && [ "$WARN_COUNT" -eq 0 ]; then
    echo -e "${C_BOLD}${C_GREEN} [PERFECT] All development requirements and tooling are ready! ${C_RESET}"
    echo -e " Start development with: ${C_BOLD}./scripts/dev-start.sh${C_RESET} or ${C_BOLD}make dev${C_RESET}"
elif [ "$ALL_PASSED" -eq 1 ]; then
    echo -e "${C_BOLD}${C_YELLOW} [READY WITH WARNINGS] Minimum requirements satisfied ($WARN_COUNT warnings). ${C_RESET}"
    echo -e " Start development with: ${C_BOLD}./scripts/dev-start.sh${C_RESET}"
else
    echo -e "${C_BOLD}${C_RED} [ACTION REQUIRED] Some essential tools are missing. Review items above. ${C_RESET}"
fi
echo -e "${C_BOLD}${C_CYAN}============================================================${C_RESET}"
echo ""
