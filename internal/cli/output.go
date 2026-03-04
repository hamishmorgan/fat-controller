package cli

import (
	"fmt"
	"os"
)

func debug(globals *Globals, format string, args ...any) {
	if !globals.Verbose {
		return
	}
	fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}
