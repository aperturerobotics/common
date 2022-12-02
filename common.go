package tscommon

// _ enables embedding
import _ "embed"

// TsConfig is the tsconfig.json embedded as a string.
//
//go:embed tsconfig.json
var TsConfig string
