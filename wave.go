package main

import (
	"github.com/neagix/Go-SDL/sound"
	"encoding/binary"
	"os"
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

func NewWaveform(file string, fmt sound.AudioInfo) *Waveform {
	wave := &Waveform{rate: uint(fmt.Rate)}
	wave.cache = mkcache(1024*1024, 2, "/home/sqweek/.cache/scribe")
	go wave.decode(file, fmt)

	return wave
}

func (wav *Waveform) decode(input string, fmt sound.AudioInfo) {
	f, _ := os.Create(wav.cache.file)
	defer f.Close()
	sample := sound.NewSampleFromFile(input, &fmt, 1024*1024)
	wav.NSamples = 0
	n := sample.Decode()
	for n > 0 {
		samps := sample.Buffer_int16()
		wav.updateMax(samps)
		binary.Write(f, binary.LittleEndian, samps)
		wav.NSamples += uint64(n)
		n = sample.Decode()
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
	if nc < ns {
		sN = s0 + nc
	} else if nc > ns {
		cN = c0 + ns
	}
	copy(samples[s0:sN], chunk.Data[c0:cN])
}

func (ww *Waveform) Samples(i0, iN uint64) []int16 {
	chunk0, chunkN := ww.cache.Bounds(i0, iN)
	chunks := make(chan *Chunk, chunkN - chunk0 + 1)
	samples := make([]int16, iN - i0)
	for id := chunk0; id <= chunkN; id++ {
		go func(i uint64) { chunks <- ww.cache.Wait(i) }(id)
	}
	for i := 0; i < int(chunkN - chunk0 + 1); i++ {
		chunk := <-chunks
		chunk.copy(samples, i0)
	}
	return samples
}

func (ww *Waveform) GetSamples(i0, iN uint64) []int16 {
	chunk0, chunkN := ww.cache.Bounds(i0, iN)
	samples := make([]int16, iN - i0)
	for chunkI := chunk0; chunkI <= chunkN; chunkI++ {
		chunk := ww.cache.Get(chunkI)
		if chunk != nil {
			chunk.copy(samples, i0)
		}
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