#!/usr/bin/env bash
set -eu

cd "$(dirname "$0")"
mkdir -p cli

for name in setup connect disconnect noreconnect; do
  GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "cli/${name}.exe" .
done
