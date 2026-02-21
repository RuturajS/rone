/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package main

import (
	"os"

	"github.com/RuturajS/rone/cmd"
)

// version is set via ldflags at build time.
var version = "dev"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

