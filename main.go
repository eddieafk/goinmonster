package main

import (
	"fmt"
	"os"

	cmd "github.com/eddieafk/goinmonster/cmd/goinmonster"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		if err := cmd.RunInit(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "generate", "gen":
		if err := cmd.RunGenerate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version", "-v", "--version":
		fmt.Printf("goinmonster version %s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`goinmonster - GraphQL to SQL code generator

Usage:
  goinmonster <command> [options]

Commands:
  init        Initialize a new goinmonster project with config file
  generate    Generate Go code from GraphQL schema (alias: gen)
  version     Print version information
  help        Show this help message

Examples:
  goinmonster init
  goinmonster generate
  goinmonster gen --config goinmonster.yaml

Configuration:
  By default, goinmonster looks for 'goinmonster.yaml' in the current directory.
  Use --config to specify a different configuration file.`)
}
