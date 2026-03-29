#!/usr/bin/env bash
# Pulls latest DB from GitHub, starts the server,
# then saves any DB changes back to GitHub on exit.

PORT=8080
DB=./data/financial.db

echo "==> Syncing from GitHub..."
git pull origin main --rebase || echo "Warning: could not pull (offline?). Starting anyway."

mkdir -p data inbox processed

echo "==> Starting server at http://localhost:$PORT — press Ctrl+C to stop"
go run ./cmd/server -db "$DB" -inbox ./inbox -port "$PORT"

echo ""
echo "==> Saving changes to GitHub..."
git add "$DB"
if git diff --cached --quiet; then
    echo "No DB changes to save."
else
    git commit -m "sync $(date '+%Y-%m-%d %H:%M')"
    git push origin main || echo "Warning: push failed. Run: git pull --rebase && git push"
fi
echo "Done."
