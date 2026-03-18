#!/usr/bin/env bash
# Fetch a specific OpenAPI spec from notip-infra at a given tag, generate TypeScript types, and commit.
#
# Usage:
#   npm run generate:openapi -- --tag v1.2.3 --file my-api.yaml
#
# Arguments:
#   --tag   Git tag or branch in notip-infra (required)
#   --file  Filename inside api-contracts/openapi/ in notip-infra (required)
set -euo pipefail

REPO="notipswe/notip-infra"
REMOTE_BASE="api-contracts/openapi"
LOCAL_DIR="api-contracts/openapi"
OUT_DIR="src/generated/openapi"

TAG=""
FILE=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --tag)  TAG="$2";  shift 2 ;;
    --file) FILE="$2"; shift 2 ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

[[ -z "$TAG"  ]] && { echo "Error: --tag is required";  exit 1; }
[[ -z "$FILE" ]] && { echo "Error: --file is required"; exit 1; }

mkdir -p "$LOCAL_DIR" "$OUT_DIR"

echo "Fetching ${FILE} from ${REPO}@${TAG}..."
gh api "repos/${REPO}/contents/${REMOTE_BASE}/${FILE}?ref=${TAG}" \
  --jq '.content' \
  | tr -d '\n' \
  | base64 -d > "${LOCAL_DIR}/${FILE}"
echo "  Saved → ${LOCAL_DIR}/${FILE}"

NAME="${FILE%.*}"
OUTPUT="${OUT_DIR}/${NAME}.ts"

echo "Generating TypeScript types → ${OUTPUT}"
npx openapi-typescript "${LOCAL_DIR}/${FILE}" -o "${OUTPUT}"

echo ""
echo "Committing..."
git add "${LOCAL_DIR}/${FILE}" "${OUTPUT}"
git commit "${LOCAL_DIR}/${FILE}" "${OUTPUT}" -m "chore(contracts): fetch openapi ${FILE} from notip-infra@${TAG}"

echo "Done."
