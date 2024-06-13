package bundle

import "container/list"

type PieceCache struct {
	queue *list.List
	items map[int64]*CacheNode
	capacity int
}

type CacheNode struct {
	val []byte
	keyPtr *list.Element
}

func NewPieceCache(capacity int) *PieceCache {
	return &PieceCache{queue: list.New(), items: make(map[int64]*CacheNode), capacity: capacity}
}

func (pc *PieceCache) Contains(pieceIndex int64) bool {
	_, ok := pc.items[pieceIndex]
	return ok
}

func (pc *PieceCache) Put(pieceIndex int64, bytes []byte) {
	if item, ok := pc.items[pieceIndex]; !ok {
		if pc.capacity == len(pc.items) {
			back := pc.queue.Back()
			pc.queue.Remove(back)
			delete(pc.items, back.Value.(int64))
		}
		pc.items[pieceIndex] = &CacheNode{val: bytes, keyPtr: pc.queue.PushFront(pieceIndex)}
	} else {
		item.val = bytes
		pc.items[pieceIndex] = item
		pc.queue.MoveToFront(item.keyPtr)
	}
}

func (pc *PieceCache) Get(pieceIndex int64) []byte {
	if item, ok := pc.items[pieceIndex]; ok {
		pc.queue.MoveToFront(item.keyPtr)
		return item.val
	}
	return nil
}