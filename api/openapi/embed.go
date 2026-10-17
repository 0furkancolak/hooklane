// Package openapi embeds the OpenAPI description of the management API.
package openapi

import _ "embed"

//go:embed openapi.yaml
var Spec []byte
