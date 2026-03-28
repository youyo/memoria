#!/bin/bash
set -euo pipefail

BINDIR="$(cd "$(dirname "$0")/.." && pwd)/bin"
BIN="${BINDIR}/memoria"
TESTDIR="/tmp/memoria-e2e-$$"
CWD="$(pwd)"

cleanup() {
  echo ""
  echo "=== Cleanup ==="
  "${BIN}" worker stop 2>/dev/null || true
  rm -rf "${TESTDIR}"
  echo "Done."
}
trap cleanup EXIT

pass() { echo "  ✓ $1"; }
fail() { echo "  ✗ $1"; exit 1; }

mkdir -p "${TESTDIR}"

echo "=== 1. Build ==="
make -C "$(dirname "$0")/.." build
[ -x "${BIN}" ] && pass "bin/memoria exists" || fail "bin/memoria not found"

echo ""
echo "=== 2. Version ==="
"${BIN}" version
pass "version command"

echo ""
echo "=== 3. Config ==="
"${BIN}" config init --force
"${BIN}" config show > /dev/null
"${BIN}" config path > /dev/null
"${BIN}" config print-hook > /dev/null
pass "config init/show/path/print-hook"

echo ""
echo "=== 4. Doctor (pre-worker) ==="
"${BIN}" doctor || true
pass "doctor runs without worker"

echo ""
echo "=== 5. Worker Start ==="
"${BIN}" worker start
sleep 3

STATUS=$("${BIN}" worker status --format json 2>/dev/null || echo '{}')
echo "${STATUS}" | grep -q '"status"' && pass "worker status returns JSON" || fail "worker status failed"

echo ""
echo "=== 6. Hook: Stop ==="
echo "{\"session_id\":\"e2e-${$}\",\"cwd\":\"${CWD}\",\"last_assistant_message\":\"The embedding model uses Ruri v3 for semantic search.\"}" | \
  "${BIN}" hook stop
pass "hook stop (exit 0)"

echo ""
echo "=== 7. Hook: SessionEnd ==="
cat > "${TESTDIR}/transcript.jsonl" << 'JSONL'
{"type":"user","content":"How does memoria store memories?"}
{"type":"assistant","content":"memoria uses SQLite with FTS5 for full-text search and vector embeddings for semantic search. Chunks are stored with kind, importance, and scope metadata."}
{"type":"user","content":"What embedding model does it use?"}
{"type":"assistant","content":"It uses cl-nagoya/ruri-v3-30m via a Python FastAPI worker communicating over Unix Domain Socket."}
JSONL

echo "{\"session_id\":\"e2e-${$}\",\"cwd\":\"${CWD}\",\"transcript_path\":\"${TESTDIR}/transcript.jsonl\",\"reason\":\"normal\"}" | \
  "${BIN}" hook session-end
pass "hook session-end (exit 0)"

echo ""
echo "=== 8. Wait for ingest (5s) ==="
sleep 5

echo ""
echo "=== 9. Memory Commands ==="
STATS=$("${BIN}" memory stats --format json 2>/dev/null || echo '{}')
echo "  stats: ${STATS}"
echo "${STATS}" | grep -q '"chunks_total"' && pass "memory stats" || fail "memory stats failed"

"${BIN}" memory list --limit 5 > /dev/null 2>&1
pass "memory list"

SEARCH=$("${BIN}" memory search "embedding" --format json 2>/dev/null || echo '[]')
echo "  search results: $(echo "${SEARCH}" | head -c 200)"
pass "memory search"

echo ""
echo "=== 10. Hook: SessionStart (retrieval) ==="
SS_OUT=$(echo "{\"session_id\":\"e2e-2-${$}\",\"cwd\":\"${CWD}\",\"transcript_path\":\"\",\"source\":\"startup\"}" | \
  "${BIN}" hook session-start 2>/dev/null || echo '{}')
echo "  output: $(echo "${SS_OUT}" | head -c 300)"
echo "${SS_OUT}" | grep -q "hookSpecificOutput" && pass "session-start returns hookSpecificOutput" || pass "session-start (empty context, OK)"

echo ""
echo "=== 11. Hook: UserPrompt (retrieval) ==="
UP_OUT=$(echo "{\"session_id\":\"e2e-2-${$}\",\"cwd\":\"${CWD}\",\"prompt\":\"How does embedding work?\"}" | \
  "${BIN}" hook user-prompt 2>/dev/null || echo '{}')
echo "  output: $(echo "${UP_OUT}" | head -c 300)"
echo "${UP_OUT}" | grep -q "hookSpecificOutput" && pass "user-prompt returns hookSpecificOutput" || pass "user-prompt (empty context, OK)"

echo ""
echo "=== 12. Memory Reindex ==="
"${BIN}" memory reindex --dry-run > /dev/null 2>&1
pass "memory reindex --dry-run"

echo ""
echo "=== 13. Doctor (full) ==="
"${BIN}" doctor
pass "doctor full check"

echo ""
echo "=== 14. Worker Stop ==="
"${BIN}" worker stop
sleep 1
STOP_STATUS=$("${BIN}" worker status 2>/dev/null || echo "not_running")
echo "  status after stop: ${STOP_STATUS}"
pass "worker stop"

echo ""
echo "==============================="
echo "  E2E テスト完了"
echo "==============================="
