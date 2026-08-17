#!/usr/bin/env bash
# ==============================================================================
# DiscoPanel Dual-Mode Rapid Development Launcher (Bash)
# ==============================================================================
# Runs Go backend and Svelte 5 frontend in tandem with trap signal handling.
# ==============================================================================

set -euo pipefail

# Script directory resolution
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

# ANSI Colors
C_RESET='\033[0m'
C_BOLD='\033[1m'
C_GREEN='\033[32m'
C_YELLOW='\033[33m'
C_RED='\033[31m'
C_CYAN='\033[36m'
C_MAGENTA='\033[35m'
C_GRAY='\033[90m'

echo ""
echo -e "${C_BOLD}${C_MAGENTA}============================================================${C_RESET}"
echo -e "${C_BOLD}${C_CYAN}    DISCOPANEL RAPID LOCAL DEVELOPMENT ENVIRONMENT          ${C_RESET}"
echo -e "${C_BOLD}${C_MAGENTA}============================================================${C_RESET}"
echo ""

# Ensure runtime directories
mkdir -p data/servers data/backups data/temp data/modules data/logs dev

# Seed DB if dev database exists and live db doesn't
if [ -f "dev/discopanel.db" ] && [ ! -f "data/discopanel.db" ]; then
    echo -e "${C_CYAN}[*] Restoring seeded development database from dev/discopanel.db...${C_RESET}"
    cp "dev/discopanel.db" "data/discopanel.db"
fi

# Default environment variables
export DISCOPANEL_SERVER_PORT="${DISCOPANEL_SERVER_PORT:-8080}"
export DISCOPANEL_DATABASE_PATH="${DISCOPANEL_DATABASE_PATH:-./data/discopanel.db}"
export DISCOPANEL_STORAGE_DATA_DIR="${DISCOPANEL_STORAGE_DATA_DIR:-./data}"
export DISCOPANEL_STORAGE_BACKUP_DIR="${DISCOPANEL_STORAGE_BACKUP_DIR:-./data/backups}"
export DISCOPANEL_STORAGE_TEMP_DIR="${DISCOPANEL_STORAGE_TEMP_DIR:-./data/temp}"
export DISCOPANEL_LOGGING_FILE_PATH="${DISCOPANEL_LOGGING_FILE_PATH:-./data/logs/discopanel.log}"
export DISCOPANEL_AUTH_LOCAL_ALLOW_REGISTRATION="${DISCOPANEL_AUTH_LOCAL_ALLOW_REGISTRATION:-true}"

FRONTEND_DIR="web/discopanel"

# Ensure frontend dependencies
if [ ! -d "$FRONTEND_DIR/node_modules" ]; then
    echo -e "${C_YELLOW}[*] Installing frontend dependencies (first run)...${C_RESET}"
    (cd "$FRONTEND_DIR" && npm install)
fi

echo -e "  ${C_BOLD}${C_GREEN}-> Frontend Dev Server:${C_RESET} http://localhost:5173"
echo -e "  ${C_BOLD}${C_GREEN}-> Backend ConnectRPC :${C_RESET} http://localhost:${DISCOPANEL_SERVER_PORT}"
echo -e "  ${C_BOLD}${C_GREEN}-> Minecraft L4 Proxy :${C_RESET} localhost:25565"

# Process management
FRONTEND_PID=""
BACKEND_PID=""

cleanup() {
    echo ""
    echo -e "${C_YELLOW}[*] Stopping DiscoPanel development processes...${C_RESET}"
    if [ -n "$FRONTEND_PID" ]; then
        kill "$FRONTEND_PID" 2>/dev/null || true
    fi
    if [ -n "$BACKEND_PID" ]; then
        kill "$BACKEND_PID" 2>/dev/null || true
    fi
    wait 2>/dev/null || true
    echo -e "${C_GREEN}[+] All development processes stopped.${C_RESET}"
    exit 0
}

trap cleanup INT TERM EXIT

# Start Frontend Dev Server
echo -e "${C_CYAN}[+] Starting Svelte 5 / Vite Dev Server...${C_RESET}"
(cd "$FRONTEND_DIR" && npm run dev) &
FRONTEND_PID=$!

sleep 1

# Start Backend Daemon
if command -v air >/dev/null 2>&1; then
    echo -e "${C_CYAN}[+] Starting Go Backend with Air live reload...${C_RESET}"
    air &
    BACKEND_PID=$!
else
    echo -e "${C_CYAN}[+] Starting Go Backend (go run cmd/discopanel/main.go)...${C_RESET}"
    go run cmd/discopanel/main.go &
    BACKEND_PID=$!
fi

echo ""
echo -e "  ${C_GRAY}Press Ctrl+C to stop all servers.${C_RESET}"
echo ""

# Wait for both processes
wait $FRONTEND_PID $BACKEND_PID
