NIP-location
======

Encrypted Location Sharing
--------------------------

`draft` `optional`

This NIP defines an addressable event type for sharing geographic location information in an encrypted format.

Motivation for the specification is to have a simple but generic and extensible way to share encrypted location data efficiently. The event is suitable for sharing fixed locations or continuous/real-time location updates.

Geohash is used as the base unit for location because of its simplicity and good Nostr adoption. An optional accuracy field is added to indicate the confidence radius of the shared location, in cases where the length of the geohash is not sufficient. Additional metadata can be added in encrypted tags inside the content field.

The combination of sender pubkeys (known or ephemeral), receiver pubkeys (known or ephemeral), and the optional p-tag enables different privacy models.

Reason for using addressable events (with d-tag) is that location is considered a changing property of a target/object/resource and that single pubkey could have several targets. With addressable events client can request location directly, without filtering possibly long list of old locations.

## Event format

Location sharing uses addressable event of `kind:30473`, which has the following format:

```yaml
{
  "kind": 30473,
  "pubkey": "<32-bytes lowercase hex-encoded public key>",
  "created_at": <unix timestamp in seconds>,
  "tags": [
    ["p", "<32-bytes lowercase hex of recipient pubkey, optional>"],
    ["d", "<identifier, optional>"],
    ["expiration", <unix timestamp as defined in NIP-40, optional>]
  ],
  "content": "<encrypted-location>",
  "sig": "<64-bytes lowercase hex of the signature>"
}
```

### Tags

- The `d` tag, used as an identifier:
  - `["d", ""] or omitted` for a single location per pubkey
  - `["d", "<name>"]` for a named location. Value should be randomly chosen unique identifier representing the target location.

- The `p` tag, used to specify the recipient:
  - **When present**: direct message to specified pubkey
  - **When omitted**: anonymous recipient message, receivers must attempt decryption

### Content

The `content` field contains encrypted location data structured as a JSON array of tags:

```json
[
  ["g", "<geohash>"],
  ["accuracy", "<optional, accuracy radius in meters at 68% confidence level>"],
  // ...
]
```

The `g` tag is required and contains a geohash string representing the location. Geohash format is same as 'g' tag in [NIP-52](52.md).
Accuracy tag is optional and defines accuracy of the location in meters with 68% confidence (1σ) from the center of the geohash. This means there is approximately 68% probability that the true location lies within this radius from the center.
Array may contain other tags. All tags except `g` are optional. This JSON array MUST be encrypted as a string using [NIP-44](44.md) encryption.

## Example use cases

### Single location sharing

Share current location with a specific user. Content is encrypted with recipients public key.

```json
{
  "kind": 30473,
  "pubkey": "<sender pubkey>",
  "tags": [
    ["p", "<recipient pubkey>"],
    ["expiration", "1735689600"]
  ],
  "content": ENCRYPTED [
    ["g", "sjkg8wghv5u"]
  ]
}
```

### Named location sharing with a group

Share a named location with a group. Group key is generated and shared out-of-band with recipients. Content is encrypted with group's public key. 'd'-tag is randomly chosen unique identifier for the place that encrypted location data represents.

```json
{
  "kind": 30473,
  "pubkey": "<sender-pubkey>",
  "tags": [
    ["p", "<group-pubkey>"],
    ["d", "93kffs"]
  ],
  "content": ENCRYPTED [
    ["g", "sjkg8wghv5u"],
    ["name", "office"]
  ]
}
```

### Anonymous location sharing

Share location with ephemeral pubkey without revealing recipient.
Receiver pubkey is known by the sender and sender pubkey is shared out-of-band with the recipient.

```json
{
  "kind": 30473,
  "pubkey": "<ephemeral pubkey>",
  "tags": [
    ["expiration", "1735689600"]
  ],
  "content": ENCRYPTED [
    ["g", "sjkg8wghv5u"]
  ]
}
```
