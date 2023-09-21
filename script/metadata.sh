#!/bin/sh
#/ script/metadata.sh runs ./tools/cmd/metadata with the given arguments.

set -e

REPO_DIR="$(CDPATH="" cd -- "$(dirname -- "$0")/.." && pwd -P)"

(
  cd "$REPO_DIR"/tools/cmd/metadata
  go build -o "$REPO_DIR"/bin/metadata
)

exec "$REPO_DIR"/bin/metadata "$@"
