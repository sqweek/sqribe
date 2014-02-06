package main

import (
	"github.com/neagix/Go-SDL/sound"
	."fmt"
	"time"
)

type FrameN int64 // frame index or frame count
type SampleN uint64 // sample index or sample count

type Waveform struct {
	NSamples SampleN
	Channels int
	rate uint
	Max []int16 // maximum amplitudes for each channel

	cache *cache
}

func NewWaveform(file string, fmt sound.AudioInfo) (*Waveform, error) {
	wave := &Waveform{rate: uint(fmt.Rate), Channels: int(fmt.Channels), NSamples: 0}
	wave.cache = mkcache(1024*1024, 2, "/home/sqweek/.cache/scribe")
	wave.Max = make([]int16, wave.Channels)
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
			wav.NSamples += SampleN(n)
			return samps[0:n/2]
		}
		Printf("decoding finished\n")
		return []int16{}
	}
}

/* samples must not contain partial frames */
func (wav *Waveform) updateMax(samples []int16) {
	for i := 0; i < len(samples); i += wav.Channels {
		for j := 0; j < wav.Channels; j++ {
			if samples[i + j] > wav.Max[j] {
				wav.Max[j] = samples[i + j]
			} else if -samples[i + j] > wav.Max[j] {
				wav.Max[j] = -samples[i + j]
			}
		}
	}
}

func (wav *Waveform) ChannelExtents(samples []int16) []int16 {
	minMax := make([]int16, wav.Channels * 2)
	for i := 0; i < len(samples); i += wav.Channels {
		for j := 0; j < wav.Channels; j++ {
			if samples[i + j] < minMax[j*2] {
				minMax[j*2] = samples[i + j]
			} else if samples[i + j] > minMax[j*2 + 1] {
				minMax[j*2 + 1] = samples[i + j]
			}
		}
	}
	return minMax
}

func (chunk *Chunk) copy(samples []int16, i0 SampleN) {
	var c0, cN, s0, sN SampleN
	cN, sN = SampleN(len(chunk.Data)), SampleN(len(samples))
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
func (wav *Waveform) Frames(f0, fN FrameN) []int16 {
//	Println("Frames", f0, fN)
	s0, sN := wav.SampleRange(f0, fN)
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

func Extract(chunks []*Chunk, s0, sN SampleN) []int16 {
	var samples []int16 = nil
	for _, chunk := range(chunks) {
		cN := chunk.I0 + SampleN(len(chunk.Data)) - 1
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
func (wav *Waveform) GetFrames(f0, fN FrameN) []*Chunk {
	s0, sN := wav.SampleRange(f0, fN)
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

func (wav *Waveform) MaxAmp() int16 {
	m := wav.Max[0]
	for j := 1; j < len(wav.Max); j++ {
		if wav.Max[j] > m {
			m = wav.Max[j]
		}
	}
	return m
}

func (wav *Waveform) TimeAtFrame(frame FrameN) time.Duration {
	durPerFrame := time.Second / time.Duration(wav.rate)
	return time.Duration(frame) * durPerFrame
}

func (wav *Waveform) FrameAtTime(t time.Duration) FrameN {
	f := FrameN(float64(t) / float64(time.Second) * float64(wav.rate))
	if f < 0 {
		f = 0
	}
	if f >= wav.ToFrame(wav.NSamples) {
		f = wav.ToFrame(wav.NSamples) - 1
	}
	return f
}

func (wav *Waveform) ToFrame(sample SampleN) FrameN {
	return FrameN(sample / SampleN(wav.Channels))
}

func (wav *Waveform) ToSample(frame FrameN) SampleN {
	return SampleN(frame * FrameN(wav.Channels))
}

func (wav *Waveform) SampleRange(f0, fN FrameN) (SampleN, SampleN) {
	s0 := SampleN(f0 * FrameN(wav.Channels))
	sN := SampleN((fN + 1) * FrameN(wav.Channels) - 1)
	return s0, sN
}
