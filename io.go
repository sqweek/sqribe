package main

import (
	"encoding/binary"
	"time"
	"sync"
	."fmt"
	"os"
)

type cache struct {
	blocksz uint /* block size in bytes */
	sampsz uint /* number of bytes to store one sample */
	chunks map[uint64]*Chunk
	lru ChunkList

	file string /* backing filename */
	iochan chan uint64 /* list of blocks that need fetching */

	iodone *sync.Cond /* triggered whenever a block completes */
	listeners []chan *Chunk

	bytesWritten int64 /* -1 if writing has finished */
}

func mkcache(blocksz, sampsz uint, file string) *cache {
	cache := cache{blocksz: 1024*1024, sampsz: 2, file: file}
	cache.iochan = make(chan uint64, 20)
	cache.chunks = make(map[uint64]*Chunk)
	cache.listeners = make([]chan *Chunk, 0, 10)
	cache.iodone = sync.NewCond(&sync.Mutex{})
	cache.lru.max = 100
	go cache.fetcher()
	return &cache
}

func (c *cache) Write(readfn func() []int16) error {
	f, err := os.Create(c.file)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := readfn()
	for len(buf) > 0 {
		binary.Write(f, binary.LittleEndian, buf)
		c.bytesWritten += int64(len(buf)) * int64(c.sampsz)
		buf = readfn()
	}
	c.bytesWritten = -1
	return nil
}

func (c *cache) Bounds(sample0, sampleN uint64) (uint64, uint64) {
	return c.Containing(sample0), c.Containing(sampleN)
}

func (c *cache) Containing(sample uint64) uint64 {
//	return uint64(math.Floor((float64(sample) * float64(c.sampsz)) / float64(c.blocksz)))
	return (sample * uint64(c.sampsz)) / uint64(c.blocksz)
}

/* doesn't block - returns nil if chunk not in cache */
func (c *cache) Get(id uint64) *Chunk {
	/* do I/O in background */
	c.iochan <- id
	chunk, ok := c.chunks[id]
	if !ok {
		return nil
	}
	return chunk
}

/* blocks waiting for the chunk to be read */
func (c *cache) Wait(id uint64) *Chunk {
	c.iodone.L.Lock()
	defer c.iodone.L.Unlock()
	chunk := c.Get(id)
	if chunk != nil {
		return chunk
	}
	ok := false
	for !ok {
		chunk, ok = c.chunks[id]
		c.iodone.Wait()
	}
	return chunk
}

func (c *cache) fetcher() *Chunk {
	for {
		id := <-c.iochan
		if chunk, ok := c.chunks[id]; ok {
			/* chunk already in cache, no i/o necessary just bump the lru */
			c.lru.touch(chunk)
			continue
		}
		filename, offset := c.pos(id)
		if offset == -1 {
			/* block not written yet - back on the queue */
			Printf("requeing block %d\n", id)
			go func() { time.Sleep(1000 * time.Millisecond); c.iochan <- id }()
			continue
		}
		file, err := os.Open(filename)
		if err != nil {
			continue
		}
		chunk, err := readchunk(id, file, c.blocksz, c.sampsz, offset)
		Printf("read chunk %d\n", id)
		file.Close()
		if err != nil {
			continue
		}
		c.add(id, chunk)
	}
}

func (c *cache) pos(id uint64) (string, int64) {
	offset := int64(id) * int64(c.blocksz)
	if c.bytesWritten != -1 && offset + int64(c.blocksz) > c.bytesWritten {
		offset = -1; /* cache still initialising, block not written yet */
	}
	return c.file, offset
}

func (c *cache) add(id uint64, chunk *Chunk) {
	c.chunks[id] = chunk
	c.iodone.L.Lock()
	c.iodone.Broadcast()
	c.iodone.L.Unlock()
	gone := c.lru.add(chunk)
	if (gone != nil) {
		delete(c.chunks, gone.id)
	}
}

func (c *cache) broadcast(chunk *Chunk) {
	for _, l := range(c.listeners) {
		go func() {l <- chunk}()
	}
}

func (c *cache) listen(listener chan *Chunk) {
	c.listeners = append(c.listeners, listener)
}

type ChunkNode struct {
	chunk *Chunk
	next *ChunkNode
	prev *ChunkNode
}

type ChunkList struct {
	size uint
	max uint
	head *ChunkNode
	tail *ChunkNode
}

func (lru *ChunkList) add(chunk *Chunk) *Chunk {
	node := &ChunkNode{chunk: chunk, next: lru.head}
	lru.head = node
	if lru.size >= lru.max {
		gone := lru.tail
		lru.tail = gone.prev
		return gone.chunk
	}
	lru.size++
	return nil
}

func (lru *ChunkList) touch(chunk *Chunk) {
	for node := lru.head; node.next != nil; node = node.next {
		if node.chunk == chunk {
			if node.prev == nil {
				return /* already top of lru */
			}
			node.prev.next = node.next
			if node.next != nil {
				node.next.prev = node.prev
			}
			node.prev = nil
			node.next = lru.head
			lru.head = node.next
			return
		}
	}
}

func readchunk(id uint64, file *os.File, blocksz uint, sampsz uint, offset int64) (*Chunk, error) {
	_, err := file.Seek(offset, 0)
	chunk := Chunk{I0: uint64(offset)/uint64(sampsz), Data: make([]int16, blocksz/sampsz), id: id}
	err = binary.Read(file, binary.LittleEndian, chunk.Data)
	return &chunk, err
}

type Chunk struct {
	I0 uint64 //first sample's index
	Data []int16

	id uint64 //index into cache.chunks
}

