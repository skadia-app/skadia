#!/usr/bin/env bash
# Point git at the versioned hooks/ directory.
# Run once after cloning the repo; idempotent.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

git config core.hooksPath hooks
chmod +x hooks/*

echo "Git hooks installed (core.hooksPath=hooks)."
echo "Active hooks:"
ls -1 hooks/ | sed 's/^/  - /'
