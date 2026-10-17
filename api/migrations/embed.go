// Package migrations embeds the Goose SQL migrations so the admin CLI can
// apply them without the source tree.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
