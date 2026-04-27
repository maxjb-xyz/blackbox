#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}/web"
npm run build:demo

rm -rf "${REPO_ROOT}/demo/worker/public"
cp -R dist "${REPO_ROOT}/demo/worker/public"

cd "${REPO_ROOT}/demo/worker"
npx wrangler deploy
