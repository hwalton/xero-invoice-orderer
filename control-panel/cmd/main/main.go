package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/hwalton/freeride-campervans/control-panel/internal/commands"
)

func usage() {
	fmt.Println("go run main.go <command> [args...]")
}

type cmdHandler func([]string) error

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		fmt.Fprintln(os.Stderr, "no .env file found — relying on environment")
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	handlers := map[string]cmdHandler{
		"run-migrations-up": handleRunMigrationsUp,
		"reset-db-dev":      handleResetDBDev,
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	handler, ok := handlers[cmd]
	if !ok {
		usage()
		os.Exit(2)
	}

	if err := handler(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s failed: %v\n", cmd, err)
		os.Exit(1)
	}
}

func handleRunMigrationsUp(args []string) error {
	var isProd bool
	if len(args) >= 1 {
		if args[0] == "--dev" || args[0] == "-d" {
			isProd = false
		} else if args[0] == "--prod" || args[0] == "-p" {
			isProd = true
		} else {
			return fmt.Errorf("Must provide argument --dev or --prod")
		}
	}

	return commands.RunMigrationsUp(isProd)
}

func handleResetDBDev(args []string) error {
	return commands.ResetDBDev()
}
