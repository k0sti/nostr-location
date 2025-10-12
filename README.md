# Nostr Location

Everything related to Nostr, maps and locations. Mostly geohash stuff.

## Nostr Location-First events

[Current specification](doc/NostrLocation.md) for location-first Nostr location events.
It adds specification for public location events.

Open design questions in [extension](doc/extension.md) document.

Old (but valid) specification for [encrypted location sharing](doc/NIP-location.md).

### Command Line Tool NOLOC

There is command line tool [noloc](nel/README.md) for handling location-first Nostr events (both public and encrypted).

## Spotstr - Nostr Map Client

There is also [Spotstr](https://github.com/k0sti/spotstr) web client that supports location sharing with location-first events and browsing any Nostr event with geohash.

## Geohash Prefix Filter

Proposal for enabling generic and efficient geohash search with [Geohash prefix filters](https://primal.net/a/naddr1qvzqqqr4gupzq935hpa4ln755mz07tedu969pnxwgmu6hc9s9fccwmzedmqkt0ldqyfhwumn8ghj7ur4wfcxcetsv9njuetn9uq32amnwvaz7tmjv4kxz7fwv3sk6atn9e5k7tcqyenk2mmgv9eksttswfjkv6tc94nxjmr5v4ez6en0wgkkummnw3ez6un9d3shjuc4duwz4).

[Rnostr relay fork](https://github.com/k0sti/rnostr-geo) with geohash prefix search support.


