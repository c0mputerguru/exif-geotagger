package main

import (
	"flag"
	"os"

	"github.com/abpatel/exif-geotagger/pkg/logger"
)

func runPrintDB() {
	printDbCmd := flag.NewFlagSet("print-db", flag.ExitOnError)
	dbPath := printDbCmd.String("db", "db.sqlite", "Path to SQLite database")

	printDbCmd.Parse(os.Args[2:])

	if err := printDatabase(*dbPath); err != nil {
		logger.Error("Error printing database: %v", err)
		os.Exit(1)
	}
}
