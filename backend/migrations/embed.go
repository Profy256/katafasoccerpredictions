// Package migrations embeds the goose SQL migrations so the katafa binary
// carries its own schema. A deploy is one artefact; there is no separate step
// that can be skipped or pointed at the wrong database.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

// Dir is the path within FS that goose walks.
const Dir = "."
