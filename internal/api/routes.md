## ROUTES

### getInfo

Headers:

- Authorization

Body (json):

- info_hashes - list of info hashes of torrents or empty means all

Returns:

- Status
- list of all torrents with following fields:
  - Info Hash
  - PartsTotal
  - PartsComplete
  - SizeTotal
  - Upload speed
  - Download Speed
  - Seeders
  - Leechers

### addTorrent

Headers:

- Authorization

Body (json):

- torrent_file - binary for torrent file

Returns:

- Status
