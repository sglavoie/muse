package main

import (
	"flag"
	"fmt"
	"os"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

func main() {
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *version {
		fmt.Println(Version)
		os.Exit(0)
	}

	// Startup proceeds here.
}
