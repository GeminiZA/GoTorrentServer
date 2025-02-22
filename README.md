# GoTorrentServer

Torrent client with full implementation of BitTorrent Protocol 1.0 and http endpoint interface for remote torrent management

I wanted to be able to manage torrent sessions from my phone while on the move, so I built this client to interface with a website through a reverse proxy.

## Notes:

- Only supports sequential piece selection so far
- No peer exchange (PEX) yet
- No BitTorrent extensions yet (thus no magnet URIs)

## Quickstart

1. Clone the repo
2. Build using go build .
3. Run ./GoTorrentServer

## Interface

### Headers:

All endpoints need the following headers:

- Authorization ** Not implemented yet **

### Endpoints:

#### /alldata (**GET**):

- no additional parameters
- Returns json:

```
{
"time": response UTC time string,
"sessions": [
  {
    "name": name,
    "infohash_base64": base64 of 20 byte infohash,
    "bitfield_base64": bitfield of pieces the client has,
    "bitfield_length": number of bits in the bitfield,
    "timestarted": time session was started,
    "wasted": kb wasted from piece hash failures or cancelled block requests,
    "downloaded": kb downloaded since session start,
    "uploaded": kb downloaded since session start,
    "error": nil if no session error,
    "trackers": [
      {
        "url": tracker url,
        "status": working / error,
        "leechers": int,
        "seeders": int,
        "peers": int,
        "lastannounce": UTC time string
      },
      ...
    ],
    "Peers": [
      {
        "ID": peerID,
        "host": "IP:port",
        "bitfield_base64": bitfield of pieces the peer has,
        "bitfield_length": number of bits in the bitfield,
        "downrate": download rate in kbps,
        "uprate": upload rate in kbps,
        "downloaded": kb downloaded,
        "uploaded": kb uploaded,
        "connected": boolean,
        "relevence": relevence of peer 0 -> 1
      },
      ...
    ]
  },
  ...
],
}
```

#### /torrentdata (GET)

- Additional query parameters:

  infohash: hex of 20 byte info hash

- Returns json:

```
{
  "name": name,
  "infohash_base64": base64 of 20 byte infohash,
  "bitfield_base64": bitfield of pieces the client has,
  "bitfield_length": number of bits in the bitfield,
  "timestarted": time session was started,
  "wasted": kb wasted from piece hash failures or cancelled block requests,
  "downloaded": kb downloaded since session start,
  "uploaded": kb downloaded since session start,
  "error": nil if no session error,
  "trackers": [
    {
      "url": tracker url,
      "status": working / error,
      "leechers": int,
      "seeders": int,
      "peers": int,
      "lastannounce": UTC time string
    },
    ...
  ],
  "Peers": [
    {
      "ID": peerID,
      "host": "IP:port",
      "bitfield_base64": bitfield of pieces the peer has,
      "bitfield_length": number of bits in the bitfield,
      "downrate": download rate in kbps,
      "uprate": upload rate in kbps,
      "downloaded": kb downloaded,
      "uploaded": kb uploaded,
      "connected": boolean,
      "relevence": relevence of peer 0 -> 1
    },
    ...
  ]
}
```

#### /addmagnet (POST)

### Not implemented yet

- Body:

```
{
  "url": magnet url,
  "targetdir": directory to save the bundle at,
  "maxdown": max download rate in kbps,
  "maxup": max upload rate in kbps,
}
```

- Returns status only

#### /addtorrentfile (POST)

- Additional query parameters:

  - path: path to download target directory
  - start: "true" / "false"
  - maxdown: max download rate in KiB/s (optional)
  - maxup: max upload rate in KiB/s (optional)

- Body: binary metadata file

- Returns status only

#### /stoptorrent (GET)

- Additional query parameters:

  - infohash: hex of the 20 byte infohash

- Returns status only

#### /starttorrent (GET)

- Additional query parameters:

  - infohash: hex of the 20 byte infohash

- Returns status only

#### /removetorrent (GET)

- Additional query parameters:

  - infohash: hex of 20 byte infohash
  - delete: "true" / "false" whether to delete the downloaded torrent data

- Returns status only

#### /setdownrate(GET)

- Additional query parameters:

  - infohash: hex of 20 byte infohash
  - rate: max download rate in KiB/s

- Returns status only

#### /setuprate(GET)

- Additional query parameters:

  - infohash: hex of 20 byte infohash
  - rate: max upload rate in KiB/s

- Returns status only

## Contributing

### Clone the repo

```bash
git clone https://github.com/GeminiZA/GoTorrentServer
cd GoTorrentServer
```

### Build the project

```bash
go build .
```

### Run the project

```bash
./GoTorrentServer
```

### Submit a pull request

If you would like to contribute, please fork the repository and open a pull request to the 'main' branch
