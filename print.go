package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/abpatel/exif-geotagger/pkg/database"
)

func printDatabase(ctx context.Context, dbPath string) error {
	repo, err := database.Connect(dbPath)
	if err != nil {
		return fmt.Errorf("error connecting to database: %w", err)
	}
	defer repo.Close()

	entries, err := repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("error fetching entries: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(entries); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}
	return nil
}
