package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"relays/pkg/models"
)

type DB struct {
	conn *sql.DB
}

func NewDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.createTables(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return db, nil
}

func (db *DB) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS relays (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT UNIQUE NOT NULL,
		host TEXT NOT NULL,
		is_alive BOOLEAN NOT NULL DEFAULT FALSE,
		last_checked DATETIME,
		latitude REAL,
		longitude REAL,
		country TEXT,
		city TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_relays_url ON relays(url);
	CREATE INDEX IF NOT EXISTS idx_relays_host ON relays(host);
	CREATE INDEX IF NOT EXISTS idx_relays_is_alive ON relays(is_alive);
	CREATE INDEX IF NOT EXISTS idx_relays_location ON relays(latitude, longitude);

	CREATE TRIGGER IF NOT EXISTS update_relays_updated_at
	AFTER UPDATE ON relays
	FOR EACH ROW
	BEGIN
		UPDATE relays SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
	END;
	`

	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) SaveRelay(relay *models.Relay) error {
	if relay.Host == "" {
		host, err := extractHostFromURL(relay.URL)
		if err != nil {
			return fmt.Errorf("failed to extract host from URL: %w", err)
		}
		relay.Host = host
	}

	query := `
	INSERT INTO relays (url, host, is_alive, last_checked, latitude, longitude, country, city)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(url) DO UPDATE SET
		is_alive = excluded.is_alive,
		last_checked = excluded.last_checked,
		latitude = COALESCE(excluded.latitude, latitude),
		longitude = COALESCE(excluded.longitude, longitude),
		country = COALESCE(excluded.country, country),
		city = COALESCE(excluded.city, city)
	`

	result, err := db.conn.Exec(query,
		relay.URL,
		relay.Host,
		relay.IsAlive,
		relay.LastChecked,
		relay.Latitude,
		relay.Longitude,
		relay.Country,
		relay.City,
	)

	if err != nil {
		return fmt.Errorf("failed to save relay: %w", err)
	}

	if relay.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			relay.ID = int(id)
		}
	}

	return nil
}

func (db *DB) GetRelay(url string) (*models.Relay, error) {
	query := `
	SELECT id, url, host, is_alive, last_checked, latitude, longitude, country, city, created_at, updated_at
	FROM relays WHERE url = ?
	`

	row := db.conn.QueryRow(query, url)

	relay := &models.Relay{}
	var lastChecked, createdAt, updatedAt sql.NullTime

	err := row.Scan(
		&relay.ID,
		&relay.URL,
		&relay.Host,
		&relay.IsAlive,
		&lastChecked,
		&relay.Latitude,
		&relay.Longitude,
		&relay.Country,
		&relay.City,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get relay: %w", err)
	}

	if lastChecked.Valid {
		relay.LastChecked = lastChecked.Time
	}
	if createdAt.Valid {
		relay.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		relay.UpdatedAt = updatedAt.Time
	}

	return relay, nil
}

func (db *DB) GetAllRelays() ([]*models.Relay, error) {
	query := `
	SELECT id, url, host, is_alive, last_checked, latitude, longitude, country, city, created_at, updated_at
	FROM relays ORDER BY created_at DESC
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all relays: %w", err)
	}
	defer rows.Close()

	var relays []*models.Relay
	for rows.Next() {
		relay := &models.Relay{}
		var lastChecked, createdAt, updatedAt sql.NullTime

		err := rows.Scan(
			&relay.ID,
			&relay.URL,
			&relay.Host,
			&relay.IsAlive,
			&lastChecked,
			&relay.Latitude,
			&relay.Longitude,
			&relay.Country,
			&relay.City,
			&createdAt,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan relay: %w", err)
		}

		if lastChecked.Valid {
			relay.LastChecked = lastChecked.Time
		}
		if createdAt.Valid {
			relay.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			relay.UpdatedAt = updatedAt.Time
		}

		relays = append(relays, relay)
	}

	return relays, nil
}

func (db *DB) GetFunctioningRelays() ([]*models.Relay, error) {
	query := `
	SELECT id, url, host, is_alive, last_checked, latitude, longitude, country, city, created_at, updated_at
	FROM relays WHERE is_alive = TRUE ORDER BY last_checked DESC
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get functioning relays: %w", err)
	}
	defer rows.Close()

	var relays []*models.Relay
	for rows.Next() {
		relay := &models.Relay{}
		var lastChecked, createdAt, updatedAt sql.NullTime

		err := rows.Scan(
			&relay.ID,
			&relay.URL,
			&relay.Host,
			&relay.IsAlive,
			&lastChecked,
			&relay.Latitude,
			&relay.Longitude,
			&relay.Country,
			&relay.City,
			&createdAt,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan relay: %w", err)
		}

		if lastChecked.Valid {
			relay.LastChecked = lastChecked.Time
		}
		if createdAt.Valid {
			relay.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			relay.UpdatedAt = updatedAt.Time
		}

		relays = append(relays, relay)
	}

	return relays, nil
}

func (db *DB) GetGeolocatedRelays() ([]*models.Relay, error) {
	query := `
	SELECT id, url, host, is_alive, last_checked, latitude, longitude, country, city, created_at, updated_at
	FROM relays WHERE latitude IS NOT NULL AND longitude IS NOT NULL ORDER BY created_at DESC
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get geolocated relays: %w", err)
	}
	defer rows.Close()

	var relays []*models.Relay
	for rows.Next() {
		relay := &models.Relay{}
		var lastChecked, createdAt, updatedAt sql.NullTime

		err := rows.Scan(
			&relay.ID,
			&relay.URL,
			&relay.Host,
			&relay.IsAlive,
			&lastChecked,
			&relay.Latitude,
			&relay.Longitude,
			&relay.Country,
			&relay.City,
			&createdAt,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan relay: %w", err)
		}

		if lastChecked.Valid {
			relay.LastChecked = lastChecked.Time
		}
		if createdAt.Valid {
			relay.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			relay.UpdatedAt = updatedAt.Time
		}

		relays = append(relays, relay)
	}

	return relays, nil
}

func (db *DB) UpdateRelayLocation(url string, location *models.GeoLocation) error {
	query := `
	UPDATE relays
	SET latitude = ?, longitude = ?, country = ?, city = ?
	WHERE url = ?
	`

	_, err := db.conn.Exec(query,
		location.Latitude,
		location.Longitude,
		location.Country,
		location.City,
		url,
	)

	if err != nil {
		return fmt.Errorf("failed to update relay location: %w", err)
	}

	return nil
}

func (db *DB) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var totalRelays, functioningRelays, geolocatedRelays int

	err := db.conn.QueryRow("SELECT COUNT(*) FROM relays").Scan(&totalRelays)
	if err != nil {
		return nil, fmt.Errorf("failed to get total relays count: %w", err)
	}

	err = db.conn.QueryRow("SELECT COUNT(*) FROM relays WHERE is_alive = TRUE").Scan(&functioningRelays)
	if err != nil {
		return nil, fmt.Errorf("failed to get functioning relays count: %w", err)
	}

	err = db.conn.QueryRow("SELECT COUNT(*) FROM relays WHERE latitude IS NOT NULL AND longitude IS NOT NULL").Scan(&geolocatedRelays)
	if err != nil {
		return nil, fmt.Errorf("failed to get geolocated relays count: %w", err)
	}

	stats["total_relays"] = totalRelays
	stats["functioning_relays"] = functioningRelays
	stats["geolocated_relays"] = geolocatedRelays

	uniqueHosts := 0
	err = db.conn.QueryRow("SELECT COUNT(DISTINCT host) FROM relays").Scan(&uniqueHosts)
	if err == nil {
		stats["unique_hosts"] = uniqueHosts
	}

	countries := 0
	err = db.conn.QueryRow("SELECT COUNT(DISTINCT country) FROM relays WHERE country IS NOT NULL AND country != ''").Scan(&countries)
	if err == nil {
		stats["unique_countries"] = countries
	}

	return stats, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func extractHostFromURL(relayURL string) (string, error) {
	u, err := url.Parse(relayURL)
	if err != nil {
		return "", err
	}

	host := u.Host
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}

	return host, nil
}