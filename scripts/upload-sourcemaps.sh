#!/usr/bin/env bash
# Upload all JavaScript source maps from a build directory.
#
# Usage:
#   CATCH_URL=https://catch.example.com \
#   CATCH_PROJECT_KEY=ck_xxx \
#   CATCH_RELEASE=web@1.2.3 \
#   ./scripts/upload-sourcemaps.sh dist
#
# The project key is the same write-only ingest key used by the browser SDK.
set -euo pipefail

CATCH_URL="${CATCH_URL:?set CATCH_URL}"
CATCH_PROJECT_KEY="${CATCH_PROJECT_KEY:?set CATCH_PROJECT_KEY}"
CATCH_RELEASE="${CATCH_RELEASE:?set CATCH_RELEASE}"
DIST_DIR="${1:-dist}"

if [[ ! -d "$DIST_DIR" ]]; then
  echo "build directory not found: $DIST_DIR" >&2
  exit 1
fi

count=0
while IFS= read -r map; do
  bundle="${map%.map}"
  rel="${bundle#"$DIST_DIR"/}"
  echo "→ $rel"
  curl --fail --silent --show-error \
    -X POST "$CATCH_URL/api/ingest/sourcemaps" \
    -H "X-Ingest-Key: $CATCH_PROJECT_KEY" \
    -H "X-Release: $CATCH_RELEASE" \
    -H "X-File: $rel" \
    -H "Content-Type: application/json" \
    --data-binary "@$map" >/dev/null
  count=$((count + 1))
done < <(find "$DIST_DIR" -type f \( -name '*.js.map' -o -name '*.mjs.map' -o -name '*.cjs.map' \) | sort)

echo "uploaded $count source map(s) for release $CATCH_RELEASE"
