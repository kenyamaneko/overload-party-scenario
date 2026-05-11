#!/usr/bin/env bash
# generate_types.sh — data/{openapi,asyncapi}.yaml から packages/api-scenario (Go) と
# packages/api-scenario-npm (TypeScript) の型を再生成する。
#
# REST 部分は oapi-codegen / openapi-typescript、Pub/Sub 部分は
# overload-party-asyncapi-codegen-tools (common 由来) を使う。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cd "$REPO_ROOT/packages/api-scenario"
oapi-codegen -config openapi-codegen.yaml ../../data/openapi.yaml

cd "$REPO_ROOT"
asyncapi-codegen \
  --input data/asyncapi.yaml \
  --output packages/api-scenario/asyncapi_gen.go \
  --package apiscenario

cd "$REPO_ROOT"
npx --yes openapi-typescript@7 \
  data/openapi.yaml \
  --output packages/api-scenario-npm/src/openapi.gen.ts
