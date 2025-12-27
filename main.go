package main

import (
	"context"
	"os"

	"github.com/secmon-lab/hecatoncheires/pkg/cli"
)

var version = "dev"

func main() {
	if err := cli.Run(context.Background(), os.Args, version); err != nil {
		os.Exit(1)
	}
}
