package cli

import (
	"fmt"
	"os"
)

func info(globals *Globals, format string, args ...any) {
	if globals.Quiet {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func debug(globals *Globals, format string, args ...any) {
	if !globals.Verbose {
		return
	}
	fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}
