package main

import (
	"github.com/neagix/Go-SDL/sound"
	."fmt"
	"time"
)

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

func NewWaveform(file string, fmt sound.AudioInfo) (*Waveform, error) {
	wave := &Waveform{rate: uint(fmt.Rate), NSamples: 0}
	wave.cache = mkcache(1024*1024, 2, "/home/sqweek/.cache/scribe")
	sample, err := sound.NewSampleFromFile(file, &fmt, 1024*1024)
	if err != nil {
		return nil, err
	}
	go wave.cache.Write(wave.decodefn(sample))

	return wave, nil
}

func (wav *Waveform) decodefn(sample *sound.Sample) func() []int16 {
	/* returns zero-length slice at EOF */
	return func() []int16 {
		n := sample.Decode()
		if n > 0 {
			samps := sample.Buffer_int16()
			Printf("decoded %d bytes (%d samples)\n", n, len(samps))
			wav.updateMax(samps[0:n/2])
			wav.NSamples += uint64(n)
			return samps[0:n/2]
		}
		Printf("decoding finished\n")
		return []int16{}
	}
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

func (chunk *Chunk) copy(samples []int16, i0 uint64) {
	var c0, cN, s0, sN uint64
	cN, sN = uint64(len(chunk.Data)), uint64(len(samples))
	if chunk.I0 > i0 {
		s0 = chunk.I0 - i0
	} else {
		c0 = i0 - chunk.I0
	}
	nc, ns := cN - c0, sN - s0
	if nc < 0 || ns < 0 {
		return
	}
	if nc < ns {
		sN = s0 + nc
	} else if nc > ns {
		cN = c0 + ns
	}
//	Println("Chunk.copy", chunk.I0, i0, len(samples), c0, cN, s0, sN)
	copy(samples[s0:sN], chunk.Data[c0:cN])
}

/* Blocks until frames from f0 to fN (inclusive) have been read from disk */
func (wav *Waveform) Frames(f0, fN int64) []int16 {
//	Println("Frames", f0, fN)
	s0, sN := uint64(2*f0), uint64(2*fN + (2 - 1))
	samples := make([]int16, sN - s0 + 1)
	chunk0, chunkN := wav.cache.Bounds(s0, sN)
	if chunk0 == chunkN {
		/* not across a chunk boundary, no need to copy */
		chunk := wav.cache.Wait(chunk0)
		return chunk.Data[s0 - chunk.I0:sN - chunk.I0 + 1]
	}
	chunks := make(chan *Chunk, chunkN - chunk0 + 1)
	for id := chunk0; id <= chunkN; id++ {
		go func(i uint64) { chunks <- wav.cache.Wait(i) }(id)
	}
	for i := 0; i < int(chunkN - chunk0 + 1); i++ {
		chunk := <-chunks
		chunk.copy(samples, s0)
	}
	return samples
}

func Extract(chunks []*Chunk, s0, sN uint64) []int16 {
	var samples []int16 = nil
	for _, chunk := range(chunks) {
		cN := chunk.I0 + uint64(len(chunk.Data)) - 1
		if !chunk.Intersects(s0, sN) {
			continue
		}
		if s0 >= chunk.I0 && sN <= cN {
			/* samples are contained entirely within this chunk */
			i0 := s0 - chunk.I0
			return chunk.Data[i0:i0 + (sN - s0 + 1)]
		}
		if samples == nil {
			samples = make([]int16, sN - s0 + 1)
		}
		chunk.copy(samples, s0)
	}
	return samples
}

/* Gets all cached samples for frames in range f0 to fN (inclusive) */
func (wav *Waveform) GetFrames(f0, fN int64) []*Chunk {
	s0, sN := uint64(2*f0), uint64(2*fN + (2 - 1))
	chunk0, chunkN := wav.cache.Bounds(s0, sN)
	chunks := make([]*Chunk, 0, chunkN - chunk0 + 1)
	for chunkI := chunk0; chunkI <= chunkN; chunkI++ {
		chunk := wav.cache.Get(chunkI)
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func (ww *Waveform) Max() int16 {
	if ww.Lmax > ww.Rmax {
		return ww.Lmax
	} else {
		return ww.Rmax
	}
}

func (wav *Waveform) TimeAtFrame(frame int64) time.Duration {
	durPerFrame := time.Second / time.Duration(wav.rate)
	return time.Duration(frame) * durPerFrame
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
