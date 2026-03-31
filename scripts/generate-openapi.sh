#!/usr/bin/env bash
# Fetch a specific OpenAPI spec from a producer repository at a given tag.
#
# Usage:
#   make fetch-openapi OPENAPI_REPO=notipswe/some-producer OPENAPI_TAG=v1.2.3 OPENAPI_FILE=my-api.yaml
#   make fetch-openapi OPENAPI_REPO=notipswe/some-producer OPENAPI_TAG=v1.2.3 OPENAPI_FILE=openapi.yaml OPENAPI_AS=some-producer-openapi.yaml
#
# Arguments:
#   --repo  Source GitHub repository (required)
#   --tag   Git tag or branch in the source repo (required)
#   --file  Filename inside api-contracts/openapi/ in the source repo (required)
#   --as    Optional local filename to save under api-contracts/openapi/ (recommended when --file is generic)
set -euo pipefail

REMOTE_BASE="api-contracts/openapi"
LOCAL_DIR="api-contracts/openapi"

REPO=""
TAG=""
FILE=""
LOCAL_FILE=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --repo) REPO="$2"; shift 2 ;;
    --tag)  TAG="$2";  shift 2 ;;
    --file) FILE="$2"; shift 2 ;;
    --as)   LOCAL_FILE="$2"; shift 2 ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

[[ -z "$REPO" ]] && { echo "Error: --repo is required"; exit 1; }
[[ -z "$TAG"  ]] && { echo "Error: --tag is required";  exit 1; }
[[ -z "$FILE" ]] && { echo "Error: --file is required"; exit 1; }

# Default local naming avoids collisions for generic remote names like openapi.yaml.
if [[ -z "$LOCAL_FILE" ]]; then
  REPO_NAME="${REPO##*/}"
  LOCAL_FILE="${REPO_NAME}-${FILE}"
fi

mkdir -p "$LOCAL_DIR"

echo "Fetching ${FILE} from ${REPO}@${TAG}..."
gh api \
  -H "Accept: application/vnd.github.raw" \
  "repos/${REPO}/contents/${REMOTE_BASE}/${FILE}?ref=${TAG}" \
  > "${LOCAL_DIR}/${LOCAL_FILE}"

if [[ ! -s "${LOCAL_DIR}/${LOCAL_FILE}" ]]; then
  echo "Error: fetched file is empty (${LOCAL_DIR}/${LOCAL_FILE}). Check --repo/--tag/--file and repository access."
  exit 1
fi

if ! grep -Eq '^[[:space:]]*openapi[[:space:]]*:|"openapi"[[:space:]]*:' "${LOCAL_DIR}/${LOCAL_FILE}"; then
  echo "Error: fetched file does not look like an OpenAPI spec (missing top-level 'openapi' field)."
  exit 1
fi
echo "  Saved → ${LOCAL_DIR}/${LOCAL_FILE}"

echo "Done."
