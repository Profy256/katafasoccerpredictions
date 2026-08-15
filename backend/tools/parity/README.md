# Model parity fixtures

`gen.ts` runs the *TypeScript* model — `src/lib/poisson.ts` and
`src/lib/model.ts` — over a fixed set of inputs and writes its output to
`backend/internal/model/testdata/parity.json`.
`backend/internal/model/parity_test.go` then diffs the Go port against it.

The point is that the fixture is generated from the real implementation rather
than hand-copied, so it cannot drift into agreeing with a bug in the port.

## Regenerating

Needed whenever `src/lib/poisson.ts` or `src/lib/model.ts` changes. If the Go
port has not changed too, the parity test failing *is the intended alarm*: the
two implementations have diverged and one of them is now publishing different
probabilities from the other.

```bash
cd "$(git rev-parse --show-toplevel)"

STAGE=$(mktemp -d)
cp src/api/types.ts "$STAGE/types.ts"
sed "s#'../api/types'#'./types'#" src/lib/markets.ts > "$STAGE/markets.ts"
sed "s#'@/api/types'#'./types'#"  src/lib/poisson.ts > "$STAGE/poisson.ts"
cp src/lib/model.ts "$STAGE/model.ts"
cp backend/tools/parity/gen.ts "$STAGE/gen.ts"

./node_modules/.bin/tsc --module commonjs --target es2022 \
  --moduleResolution node --skipLibCheck --outDir "$STAGE/out" "$STAGE"/*.ts

node "$STAGE/out/gen.js" > backend/internal/model/testdata/parity.json
go -C backend test ./internal/model/...
```

The `sed` calls exist only because `poisson.ts` imports through the `@/` alias
and `markets.ts` through a relative path that does not survive being copied
flat. Nothing else is modified — the model code under test is byte-identical to
what ships.

## After the cutover

At Phase 8 the frontend stops generating data, but `src/lib/poisson.ts` stays:
the methodology page and the match detail view still describe this model. Keep
this harness. The moment the two implementations are allowed to drift, the
published methodology stops describing the published numbers.
