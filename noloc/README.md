# noloc - Nostr Location

noloc is a CLI tool for handling location-first Nostr events including both public location events (kind 30472) and encrypted location events (kind 30473).

## Features

- 🔐 Optional end-to-end encrypted location sharing using NIP-44
- 📍 Geohash-based location encoding
- 🆔 Multiple identity management
- 🛰️ ISS tracker demo application
- 📡 Real-time location event listener
- 📍 Support for both public (kind 30472) and encrypted (kind 30473) location events

## Installation

```bash
go install ./...
# or
just build
```

## Usage

### ISS Tracker Demo

Track the International Space Station and broadcast its location:

```bash
noloc iss --sender-nsec <nsec> --receiver-npub <npub> --relay <relay-url>
```

### Listen for Location Events

Receive and decrypt location messages:

```bash
noloc listen --receiver-nsec <nsec> --relay <relay-url>
```

### Identity Management

```bash
# Generate new identity
noloc id generate alice

# List identities
noloc id list

# Show identity details
noloc id show alice
```

## Configuration

noloc supports configuration via:
- Command-line flags
- Environment variables (NOLOC_ prefix)
- .env file
- ~/.noloc.yaml config file

## NIP-location Specification

This implementation follows the location-first event specifications defined in NIP-location.md, using:
- Event kind 30472 for public location events
- Event kind 30473 for encrypted location events with NIP-44 encryption
- Geohash encoding for coordinates