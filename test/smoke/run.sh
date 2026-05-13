#!/usr/bin/env bash
# Prints the v1 manual smoke checklist for human verification.
# v1 acceptance is gated on this checklist, not on automated assertions —
# CI-able portions live under test/headless.
set -euo pipefail
cd "$(dirname "$0")"

cat v1.md
echo
echo "--- SFTP ---"
echo
cat sftp.md
