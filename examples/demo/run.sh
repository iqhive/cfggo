#!/usr/bin/env bash
set -euo pipefail

go build -o .demo-app .

cleanup() {
	kill "${child:-}" 2>/dev/null || true
	rm -f .demo-app
}
trap cleanup EXIT INT TERM

./.demo-app &
child=$!
wait "$child"
