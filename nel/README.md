# NEL - Nostr Encrypted Location

NEL is a CLI tool for sharing encrypted location data over the Nostr protocol using NIP-44 encryption.

## Features

- 🔐 End-to-end encrypted location sharing using NIP-44
- 📍 Geohash-based location encoding  
- 🆔 Multiple identity management
- 🛰️ ISS tracker demo application
- 📡 Real-time location event listener

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
nel iss --sender-nsec <nsec> --receiver-npub <npub> --relay <relay-url>
```

### Listen for Location Events

Receive and decrypt location messages:

```bash
nel listen --receiver-nsec <nsec> --relay <relay-url>
```

### Identity Management

```bash
# Generate new identity
nel id generate --save alice

# List identities  
nel id list

# Show identity details
nel id show alice
```

## Configuration

NEL supports configuration via:
- Command-line flags
- Environment variables (NEL_ prefix)
- .env file
- ~/.nel.yaml config file

## NIP-location Specification

This implementation follows the encrypted location sharing specification defined in NIP-location.md, using:
- Event kind: 30473
- NIP-44 encryption for content
- Geohash encoding for coordinates