# Relays - Nostr Relay Discovery and Geolocation Tool

> Based on: [georelays](https://github.com/permissionlesstech/georelays)

A Go application that discovers Nostr relays and determines their geographic locations based on IP addresses. This is a Go port of the Python-based georelays project.

## Features

- **Relay Discovery**: Crawls Nostr relays using breadth-first search through follow lists (kind 3) and relay lists (kind 10002)
- **IP Geolocation**: Determines relay locations using DB-IP database
- **SQLite Storage**: Persistent storage of relay data and locations
- **Concurrent Processing**: Efficient batch processing with configurable concurrency
- **Export Support**: Export data to JSON or CSV formats

## Components

### 1. Relay Crawler (`internal/crawler`)
- Connects to Nostr relays via WebSocket
- Extracts relay URLs from Nostr events (tags `r` and `p`)
- Tests relay responsiveness
- Performs breadth-first discovery

### 2. Geolocator (`internal/geolocator`)
- Downloads and processes DB-IP IPv4 geolocation database
- Resolves relay hostnames to IP addresses
- Binary search for efficient IP range lookup
- Returns latitude, longitude, country, and city information

### 3. Database (`internal/database`)
- SQLite database for persistent storage
- Stores relay URLs, status, and location data
- Provides query methods for statistics and exports

## Installation

```bash
cd relays
go mod tidy
go build -o relays cmd/main.go
```

## Usage

### Full Discovery and Geolocation
```bash
./relays full --seed wss://relay.damus.io --depth 3 --batch 10
```

### Discovery Only
```bash
./relays discover --seed wss://relay.damus.io --depth 2
```

### Geolocation Only (for existing relays)
```bash
./relays geolocate
```

### View Statistics
```bash
./relays stats
```

### Export Data
```bash
# Export to JSON
./relays export --output relays.json

# Export to CSV
./relays export --output relays.csv
```

## Configuration Options

- `--db`: SQLite database path (default: "relays.db")
- `--seed`: Seed relay URL (default: "wss://relay.damus.io")
- `--depth`: Maximum discovery depth (default: 3)
- `--batch`: Batch size for concurrent processing (default: 10)
- `--timeout`: Timeout for relay connections (default: 10s)
- `--output`: Output file for exports

## Environment Variables

All flags can be set via environment variables with the `RELAYS_` prefix:
- `RELAYS_DB`
- `RELAYS_SEED`
- `RELAYS_DEPTH`
- `RELAYS_BATCH`
- `RELAYS_TIMEOUT`

## Database Schema

The SQLite database contains a `relays` table with the following columns:
- `id`: Primary key
- `url`: Relay WebSocket URL
- `host`: Extracted hostname
- `is_alive`: Whether the relay is responsive
- `last_checked`: Last time the relay was tested
- `latitude`, `longitude`: Geographic coordinates
- `country`, `city`: Location information
- `created_at`, `updated_at`: Timestamps

## Geolocation Data

This tool uses the DB-IP database for geolocation, which provides:
- IPv4 address ranges mapped to geographic coordinates
- Country and city information
- Accuracy note: Geolocation reflects ISP/hosting provider location, not precise server location

## Attribution

Geolocation data is provided by DB-IP, available at https://www.db-ip.com.