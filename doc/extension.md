# Extensions & Proposals for Location Event Specificataion

Some design thoughts for additional location tags.

## Tag: altitude

Location altitude in meters. This would be used in combination with geohash or lat/long coords to define altitude of the location. In meters.

## Geolocation with coordinates

There should be standard way to define location with latitude and longitude.
`location`-tag defined in [NIP-52](https://github.com/nostr-protocol/nips/blob/master/52.md) may contain GPS coordinates, but exact format is not specified.
Common way is to specify coords as `[latitude, longitude]`. GeoJSON has different ordering:  `[longitude, latitude]`.

Possibilities:
- Add type sepcification in `location` tag array
```
["location", "40.7128,-74.0060", "wgs84"]
["location", "+27.5916+086.5640+8850CRSWGS_84/", "iso6709"]
```
- Latitude and longitude as separate tags
```
["lat", "40.7128"]
["lon", "-74.0060"]
```
- Options for combined latitude and longitude tags: `coord`, `wgs84`, `latlon`, `lonlat`.
- Full names or abbreviations? `lat` vs `latitude`.

