#!/usr/bin/env bash
set -euo pipefail

FIRESTORE_PORT=9041
GOOGLE_CLOUD_PROJECT_ID=overload-party-test
HEALTH_CHECK_TIMEOUT_SEC=30

gcloud emulators firestore start --host-port="localhost:${FIRESTORE_PORT}" >/tmp/firestore.log 2>&1 &
is_started=false
for _ in $(seq 1 "${HEALTH_CHECK_TIMEOUT_SEC}"); do
  if curl -sf "http://localhost:${FIRESTORE_PORT}" >/dev/null; then
    is_started=true
    break
  fi
  sleep 1
done
if [ "$is_started" = false ]; then
  echo "Firestore emulator failed to start"
  cat /tmp/firestore.log
  exit 1
fi

# Testcontainers が起動する image は実行中に自動 pull されるが、大きな image は
# テストの startup timeout を超えうるため事前に取得しておく
docker pull postgres:16-alpine
docker pull gcr.io/google.com/cloudsdktool/cloud-sdk:emulators

{
  echo "FIRESTORE_EMULATOR_HOST=localhost:${FIRESTORE_PORT}"
  echo "GOOGLE_CLOUD_PROJECT_ID=${GOOGLE_CLOUD_PROJECT_ID}"
} >>"$GITHUB_ENV"
