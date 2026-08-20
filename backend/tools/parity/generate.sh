#!/usr/bin/env bash
#
# Regenerates backend/internal/model/testdata/parity.json by running the
# *TypeScript* model — frontend/src/lib/poisson.ts and model.ts — over a fixed
# set of inputs. backend/internal/model/parity_test.go then diffs the Go port
# against it.
#
# The fixture is generated from the real implementation rather than hand-copied,
# so it cannot drift into agreeing with a bug in the port.
#
# Run from anywhere:
#   ./backend/tools/parity/generate.sh
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
FRONTEND="$ROOT/frontend"
OUT="$ROOT/backend/internal/model/testdata/parity.json"

if [ ! -x "$FRONTEND/node_modules/.bin/tsc" ]; then
  echo "TypeScript is not installed. Run 'npm ci' in $FRONTEND first." >&2
  exit 1
fi

# Staged *inside* frontend/ rather than in /tmp so that TypeScript resolves
# @types/node by walking up into frontend/node_modules. A stage directory
# outside the project fails with "Cannot find name 'process'".
STAGE=$(mktemp -d "$FRONTEND/.parity-stage.XXXXXX")
trap 'rm -rf "$STAGE"' EXIT

# The model sources are copied flat and their imports rewritten, because
# poisson.ts imports through the '@/' alias and markets.ts through a relative
# path — neither survives being copied out of the Next.js project. Nothing else
# is modified: the model code under test is byte-identical to what ships.
cp "$FRONTEND/src/api/types.ts"                                "$STAGE/types.ts"
sed "s#'../api/types'#'./types'#" "$FRONTEND/src/lib/markets.ts" > "$STAGE/markets.ts"
sed "s#'@/api/types'#'./types'#"  "$FRONTEND/src/lib/poisson.ts" > "$STAGE/poisson.ts"
cp "$FRONTEND/src/lib/model.ts"                                "$STAGE/model.ts"
cp "$ROOT/backend/tools/parity/gen.ts"                         "$STAGE/gen.ts"

# Run from inside the stage: with no tsconfig, TypeScript looks for @types
# relative to the *working directory*, and the stage sits under frontend/ so
# the walk up reaches frontend/node_modules/@types. Invoking it from the repo
# root instead fails on 'process' being undefined.
(
  cd "$STAGE"
  ./../node_modules/.bin/tsc \
    --module commonjs --target es2022 \
    --moduleResolution node --skipLibCheck \
    --outDir out ./*.ts
)

node "$STAGE/out/gen.js" > "$OUT"
echo "wrote $OUT"
