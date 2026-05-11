#!/usr/bin/env bash
# 算出済みタグから npm パッケージを build し package.json に version を反映する。
# 入力: TAG (env、prefix `packages/api-scenario-npm/v` を含む完全タグ名)、PWD は packages/api-scenario-npm/。
# data/openapi.yaml から openapi-typescript で src/openapi.gen.ts を生成してから tsc を回す。
set -euo pipefail

: "${TAG:?TAG env required}"

version="${TAG#packages/api-scenario-npm/v}"
npx --yes openapi-typescript@7 ../../data/openapi.yaml --output src/openapi.gen.ts
npm install
npm version "${version}" --no-git-tag-version
npm run build
