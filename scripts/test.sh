#!/usr/bin/env bash

set -euo pipefail

go test ./...
./scripts/e2e.sh
