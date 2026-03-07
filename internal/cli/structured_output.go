package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
)

func writeStructured(out io.Writer, format string, v any) error {
	switch format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case "toml":
		return toml.NewEncoder(out).Encode(v)
	default:
		return fmt.Errorf("unsupported structured format: %q", format)
	}
}

func isStructuredOutput(globals *Globals) bool {
	return globals != nil && (globals.Output == "json" || globals.Output == "toml")
}
