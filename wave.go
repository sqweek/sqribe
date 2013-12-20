package main

import (
	"github.com/neagix/Go-SDL/sound"
	"encoding/binary"
	"os"
)

type cache struct {
	blocksz uint /* block size in bytes */
	sampsz uint /* number of bytes to store one sample */
	nblocks uint64 /* total number of blocks */
	chunks map[uint64]*Chunk
	lru ChunkList

	file string /* backing filename */
	iochan chan uint64 /* list of blocks that need fetching */
}

func mkcache(blocksz, sampsz uint, file string) *cache {
	cache := cache{blocksz: 1024*1024, sampsz: 2, file: file}
	cache.iochan = make(chan uint64)
	cache.chunks = make(map[uint64]*Chunk)
	cache.lru.max = 100
	go cache.fetcher()
	return &cache
}

func (c *cache) containing(sample uint64) uint64 {
	return (sample * uint64(c.sampsz)) / uint64(c.blocksz)
}

/* doesn't block - returns nil if chunk not in cache */
func (c *cache) get(i uint64) *Chunk {
	/* do I/O in background */
	go func() { c.iochan <- i }()
	chunk, ok := c.chunks[i]
	if !ok {
		return nil
	}
	return chunk
}

func (c *cache) fetcher() *Chunk {
	for {
		i := <-c.iochan
		if chunk, ok := c.chunks[i]; ok {
			/* chunk already in cache, no i/o necessary just bump the lru */
			c.lru.touch(chunk)
			continue
		}
		filename, offset := c.pos(i)
		file, err := os.Open(filename)
		if err != nil {
			continue
		}
		chunk, err := readchunk(i, file, c.blocksz, c.sampsz, offset)
		file.Close()
		if err != nil {
			continue
		}
		c.add(i, chunk)
	}
}

func (c *cache) pos(i uint64) (string, uint64) {
	return c.file, i * uint64(c.blocksz)
}

func (c *cache) add(i uint64, chunk *Chunk) {
	c.chunks[i] = chunk
	gone := c.lru.add(chunk)
	if (gone != nil) {
		delete(c.chunks, gone.i)
	}
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
			if node.prev != nil {
				node.prev.next = node.next
			} else {
				return /* already top of lru */
			}
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

func readchunk(i uint64, file *os.File, blocksz uint, sampsz uint, offset uint64) (*Chunk, error) {
	_, err := file.Seek(int64(offset), 0)
	chunk := Chunk{I0: offset/uint64(sampsz), Data: make([]int16, blocksz/sampsz), i: i}
	err = binary.Read(file, binary.LittleEndian, chunk.Data)
	return &chunk, err
}

type Chunk struct {
	I0 uint64 //first sample's index
	Data []int16

	i uint64 //index into cache.chunks
}

type Waveform struct {
	NSamples uint64
	rate uint
	Lmax int16 // left channel maximum amplitude
	Rmax int16 // right channel maximum amplitude

	cache *cache
}

type WaveRange struct {
	min int16
	max int16
}

func max(a, b int16) int16 {
	if a > b {
		return a
	}
	return b
}

func min(a, b int16) int16 {
	if a < b {
		return a
	}
	return b
}

func NewWaveform(file string, fmt sound.AudioInfo) Waveform {
	wave := Waveform{rate: uint(fmt.Rate)}
	wave.cache = mkcache(1024*1024, 2, "/home/sqweek/.cache/scribe")
	wave.decode(file, fmt)

	return wave
}

func (wav *Waveform) decode(input string, fmt sound.AudioInfo) {
	f, _ := os.Create(wav.cache.file)
	sample := sound.NewSampleFromFile(input, &fmt, 1024*1024)
	wav.NSamples = 0
	n := sample.Decode()
	for n > 0 {
		wav.NSamples += uint64(n)
		samps := sample.Buffer_int16()
		wav.updateMax(samps)
		binary.Write(f, binary.LittleEndian, samps)
		n = sample.Decode()
	}
	f.Close()
}

func (wav *Waveform) updateMax(samples []int16) {
	left, right := WaveRanges(samples)
	lmax := max(left.max, -left.min)
	rmax := max(right.max, -right.min)
	if lmax > wav.Lmax {
		wav.Lmax = lmax
	}
	if rmax > wav.Rmax {
		wav.Rmax = rmax
	}
}

func (ww *Waveform) Samples(i0, iN uint64) []*Chunk {
	chunk0 := ww.cache.containing(i0)
	chunkN := ww.cache.containing(iN)
	chunks := make([]*Chunk, 0, chunkN - chunk0)
	for chunkI := chunk0; chunkI <= chunkN; chunkI++ {
		chunk := ww.cache.get(chunkI)
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func (ww *Waveform) GetSamples(i0, iN uint64) []int16 {
	chunks := ww.Samples(i0, iN)
	samples := make([]int16, iN-i0)
	for _, chunk := range chunks {
		var c0, cN, s0, sN uint64
		cN, sN = uint64(len(chunk.Data)), uint64(len(samples))
		if chunk.I0 > i0 {
			s0 = chunk.I0 - i0
		} else {
			c0 = i0 - chunk.I0
		}
		nc, ns := cN - c0, sN - s0
		if nc < ns {
			sN = s0 + nc
		} else if nc > ns {
			cN = c0 + ns
		}
		copy(samples[s0:sN], chunk.Data[c0:cN])
	}
	return samples
}

func (ww *Waveform) Max() int16 {
	if ww.Lmax > ww.Rmax {
		return ww.Lmax
	} else {
		return ww.Rmax
	}
}

func WaveRanges(s []int16) (WaveRange, WaveRange) {
	if len(s) < 2 {
		return WaveRange{0,0}, WaveRange{0,0}
	}
	left := WaveRange{s[0],s[0]}
	right := WaveRange{s[1],s[1]}
	for i := 0; i < len(s); i+=2 {
		left.include(s[i])
		right.include(s[i+1])
	}
	return left, right
}

func (rng *WaveRange) include(samp int16) {
	if samp > rng.max { rng.max = samp }
	if samp < rng.min { rng.min = samp } 
}

func (r1 *WaveRange) Union(r2 *WaveRange) WaveRange {
	return WaveRange{max(r1.min, r2.min), min(r1.max, r2.max)}
}