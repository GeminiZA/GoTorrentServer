# Peer requirements

## Interface

- **Handshake**(conn, peerID, infoHash) **bool** => return if valid handshake
- **Connect**(IP, Port, peerID, infoHash) **\*Peer** => return peer object after handshake
- **RequestBlock**(pieceIndex offset) **void** => Add request to queue
- **SendBlock**(pieceIndex, offset, block) **void** => Add block to outbound queue to respond to peer request
- **CancelBlock**(pieceIndex, offset, length) **void** => Cancel block, remove from outbound queue
- **GetNextInRequest**() **\*BlockRequest** => Get next request from peer request queue
- **GetNumOutRequests**() **int** => Get number of outbound requests not yet filled
- **GetNextInPiece**() **\*BlockResponse** => Get next piece from peer response queue
- **GetCancelledBlock**() **\*BlockRequest** => Get info of timed / cancelled request

## Internal

- use one routine for all communication, parse in messages first
- Keep track of request times to time out old requests
- Handle choking / interested properly
- Check if a piece was requested and not cancelled before queuing

# Session requirements

## Interface

- **New**() **\*Session** => instantiate session and check for bundle existence (rehash if it exists)
- **Start**() **void** => Start tracker and connect to peers and download/upload pieces
- **Stop**() **void** => Stop tracker and disconnect from peers

## Internal

- Use seperate goroutines for each peer
- Use peer functions to communicate
- Assign whole pieces to different client if they are not choked
