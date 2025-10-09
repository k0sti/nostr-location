package models

import (
	"time"
)

type Relay struct {
	ID          int       `json:"id" db:"id"`
	URL         string    `json:"url" db:"url"`
	Host        string    `json:"host" db:"host"`
	IsAlive     bool      `json:"is_alive" db:"is_alive"`
	LastChecked time.Time `json:"last_checked" db:"last_checked"`
	Latitude    *float64  `json:"latitude,omitempty" db:"latitude"`
	Longitude   *float64  `json:"longitude,omitempty" db:"longitude"`
	Country     *string   `json:"country,omitempty" db:"country"`
	City        *string   `json:"city,omitempty" db:"city"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type DiscoveryStats struct {
	TotalRelaysFound      int           `json:"total_relays_found"`
	FunctioningRelays     int           `json:"functioning_relays"`
	EventsProcessed       int           `json:"events_processed"`
	StartTime             time.Time     `json:"start_time"`
	Duration              time.Duration `json:"duration"`
	UniqueHosts           int           `json:"unique_hosts"`
	GeolocatedRelays      int           `json:"geolocated_relays"`
}

type NostrEvent struct {
	ID        string      `json:"id"`
	Pubkey    string      `json:"pubkey"`
	Kind      int         `json:"kind"`
	Tags      [][]string  `json:"tags"`
	Content   string      `json:"content"`
	Sig       string      `json:"sig"`
	CreatedAt int64       `json:"created_at"`
}

type NostrFilter struct {
	IDs     []string `json:"ids,omitempty"`
	Authors []string `json:"authors,omitempty"`
	Kinds   []int    `json:"kinds,omitempty"`
	Since   *int64   `json:"since,omitempty"`
	Until   *int64   `json:"until,omitempty"`
	Limit   *int     `json:"limit,omitempty"`
}

type NostrRequest struct {
	Type   string        `json:"type"`
	SubID  string        `json:"sub_id"`
	Filter []NostrFilter `json:"filter"`
}

type GeoLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Country   string  `json:"country,omitempty"`
	City      string  `json:"city,omitempty"`
}