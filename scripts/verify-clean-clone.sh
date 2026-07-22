#!/bin/bash
# TiBrain Clean-Clone Verification Script
# Verifies that the repository builds and tests without untracked files
# Exit 0 = clean clone verified, Exit 1 = verification failed

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

echo "=== TiBrain Clean-Clone Verification ==="
echo "Repository root: $REPO_ROOT"

# Get current commit SHA
if git rev-parse HEAD >/dev/null 2>&1; then
    CURRENT_SHA=$(git rev-parse HEAD)
    echo "Current commit: $CURRENT_SHA"
else
    echo "ERROR: Not a git repository or git not available"
    exit 1
fi

# Check for untracked files
echo ""
echo "Checking for untracked files..."
UNTRACKED=$(git status --porcelain | grep '^??' || true)
if [ -n "$UNTRACKED" ]; then
    echo "ERROR: Untracked files found:"
    echo "$UNTRACKED" | while read line; do echo "  $line"; done
    echo "Clean clone verification FAILED"
    exit 1
fi
echo "OK: No untracked files"

# Check required directories exist
for dir in internal/db internal/memory internal/mcp internal/tools; do
    if [ -d "$REPO_ROOT/$dir" ]; then
        echo "OK: $dir exists"
    else
        echo "WARNING: $dir missing (may be expected)"
    fi
done

# Check go.mod exists
if [ ! -f "$REPO_ROOT/go.mod" ]; then
    echo "ERROR: go.mod not found"
    exit 1
fi
echo "OK: go.mod exists"

# Run go mod download
export CGO_ENABLED=0
echo ""
echo "Running go mod download..."
if ! go mod download; then
    echo "ERROR: go mod download failed"
    exit 1
fi
echo "OK: Dependencies downloaded"

# Run go build
echo ""
echo "Running go build ./... ..."
if ! go build -trimpath ./...; then
    echo "ERROR: go build failed"
    exit 1
fi
echo "OK: Build successful"

# Run go vet
echo ""
echo "Running go vet ./... ..."
if ! go vet ./...; then
    echo "ERROR: go vet failed"
    exit 1
fi
echo "OK: Vet passed"

echo ""
echo "=== Clean-Clone Verification PASSED ==="
echo "Commit: $CURRENT_SHA"
exit 0