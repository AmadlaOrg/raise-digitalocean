#!/bin/bash
set -euo pipefail
mkdir -p .reports
go test -v -coverprofile=.reports/coverage.out ./... 2>&1 | tee .reports/test-output.txt
go tool cover -func=.reports/coverage.out
