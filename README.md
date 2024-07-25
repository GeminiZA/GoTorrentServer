# GoTorrentServer

---

Torrent client with http endpoint interface for remote torrent management

## Interface
### Headers:
All endpoints need the following headers:
- Authorization
### Endpoints:
#### /alldata (**GET**):
- No additional headers
- Returns json:
```
{
"time": response UTC time string,
"sessions": [
  {
    "name": name,
	"infohash", 20 byte info hash as utf8 string,
    "pieces": bitfield of pieces the client has,
	"timestarted": time session was started,
	"wasted": kb wasted from piece hash failures or cancelled block requests,
	"downloaded": kb downloaded since session start,
	"uploaded": kb downloaded since session start,
	"trackers": [
	  "url": tracker url,
	  "status": working / error,
	  "leechers": int,
	  "seeders": int,
	  "peers": int,
	  "lastannounce": UTC time string
	],
    "Peers": [
	  {
	    "ID": peerID,
	    "host": "IP:port",
	    "pieces": bitfield of pieces the peer has,
	    "down": download rate in kbps,
	    "up": upload rate in kbps
      }
    ],
    "status": connected / disconnected etc
  }
],
}
```

#### /torrentdata (GET)

- Additional headers:
```
{
  "infohash": torrent info hash as string
}
```

- Returns json:
```
{
  "name": name,
  "infohash": 20 byte infohash as utf8 string,
  "pieces": bitfield of pieces the client has,
  "timestarted": time session was started,
  "wasted": kb wasted from piece hash failures or cancelled block requests,
  "downloaded": kb downloaded since session start,
  "uploaded": kb downloaded since session start,
  "trackers": [
  "url": tracker url,
  "status": working / error,
  "leechers": int,
  "seeders": int,
  "peers": int,
  "lastannounce": UTC time string
  ],
  "Peers": [
	{
	  "ID": peerID,
	  "host": "IP:port",
	  "pieces": bitfield of pieces the peer has,
	  "down": download rate in kbps,
	  "up": upload rate in kbps
    }
  ],
  "status": connected / disconnected etc
}
```

#### /addmagnet (POST)

- Additional headers:
```
{
  "time": UTC time string of request time,
  "content-type": "application/json" 
}
```

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
- Additional headers:
```
{
  "time": UTC time string of request time,
  "content-type": "application/json" 
}
```

- Body:
```
{
  "file": bencoded file in utf8 string,
  "targetdir": directory to save the bundle at,
  "maxdown": max download rate in kbps,
  "maxup": max upload rate in kbps,
}
```

- Returns status only 

#### /pause (GET)

- Additional headers:
```
{
  "infohash": 20 byte infohash as utf8 string
}
```

- Returns status only


#### /resume (GET)

- Additional headers:
```
{
  "infohash": 20 byte infohash as utf8 string
}
```

- Returns status only

#### /remove (GET)

- Additional headers:
```
{
  "infohash": 20 byte infohash as utf8 string,
  "deletedata": bool to delete the downloaded data also
}
```

- Returns status only

#### /setdownrate(GET)

- Additional headers:
```
{
  "infohash": infohash,
  "downrate": max download rate in kbps
}
```

- Returns status only

#### /setuprate(GET)

- Additional headers:
```
{
  "infohash": infohash,
  "uprate": max upload rate in kbps
}
```

- Returns status only