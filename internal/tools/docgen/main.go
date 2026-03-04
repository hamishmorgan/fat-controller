// Command docgen generates Markdown CLI reference documentation from the
// Kong command model. Run it via: go run ./internal/tools/docgen -out docs/cli
package main

import (
	"flag"
	"log"

	"github.com/alecthomas/kong"
	"github.com/hamishmorgan/fat-controller/internal/cli"
)

func main() {
	out := flag.String("out", "docs/cli", "output directory for generated Markdown files")
	flag.Parse()

	var c cli.CLI
	k, err := kong.New(&c,
		kong.Name("fat-controller"),
		kong.Description("CLI for managing Railway projects. Pull live config, diff against desired state, apply the difference."),
	)
	if err != nil {
		log.Fatalf("build kong model: %v", err)
	}

	opts := Options{
		AppName:        "fat-controller",
		AppDescription: k.Model.Help,
	}
	if err := Generate(k.Model, *out, opts); err != nil {
		log.Fatalf("generate docs: %v", err)
	}

	log.Printf("generated docs in %s", *out)
}
