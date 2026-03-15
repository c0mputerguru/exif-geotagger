package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type LocationEntry struct {
	Timestamp   time.Time
	Latitude    float64
	Longitude   float64
	Altitude    *float64
	City        *string
	State       *string
	Country     *string
	DeviceModel string
}

// scanLocationEntry scans a single row into a Location struct.
func scanLocationEntry(row *sql.Rows) (LocationEntry, error) {
	var e LocationEntry
	var ts time.Time
	err := row.Scan(&ts, &e.Latitude, &e.Longitude, &e.Altitude, &e.City, &e.State, &e.Country, &e.DeviceModel)
	if err != nil {
		return LocationEntry{}, err
	}
	e.Timestamp = ts
	return e, nil
}

type Repository struct {
	db *sql.DB
}

func Connect(path string) (*Repository, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return &Repository{db: db}, nil
}

func initSchema(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS locations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		latitude REAL NOT NULL,
		longitude REAL NOT NULL,
		altitude REAL,
		city TEXT,
		state TEXT,
		country TEXT,
		device_model TEXT NOT NULL,
		UNIQUE(timestamp, device_model)
	);
	CREATE INDEX IF NOT EXISTS idx_timestamp ON locations(timestamp);
	`
	_, err := db.Exec(query)
	return err
}

func (r *Repository) Insert(entry LocationEntry) error {
	query := `
	INSERT OR REPLACE INTO locations (timestamp, latitude, longitude, altitude, city, state, country, device_model)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		entry.Timestamp,
		entry.Latitude,
		entry.Longitude,
		entry.Altitude,
		entry.City,
		entry.State,
		entry.Country,
		entry.DeviceModel,
	)
	return err
}

func (r *Repository) Close() error {
	return r.db.Close()
}

// FindClosest returns the closest locations within +/- window of the target time
func (r *Repository) FindClosest(target time.Time, window time.Duration) ([]LocationEntry, error) {
	start := target.Add(-window)
	end := target.Add(window)

	query := `
	SELECT timestamp, latitude, longitude, altitude, city, state, country, device_model
	FROM locations
	WHERE timestamp >= ? AND timestamp <= ?
	ORDER BY ABS(julianday(timestamp) - julianday(?)) ASC
	`
	rows, err := r.db.Query(query, start, end, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LocationEntry
	for rows.Next() {
		e, err := scanLocationEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// GetAll returns all location entries in the database
func (r *Repository) GetAll() ([]LocationEntry, error) {
	query := `
	SELECT timestamp, latitude, longitude, altitude, city, state, country, device_model
	FROM locations
	ORDER BY timestamp ASC
	`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LocationEntry
	for rows.Next() {
		e, err := scanLocationEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}
