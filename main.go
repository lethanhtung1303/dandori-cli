package main

import (
	"fmt"
	"os"

	"github.com/phuc-nt/dandori-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
