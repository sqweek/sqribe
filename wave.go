package main

type Waveform struct {
	Samples []int16
	rate uint
	Lmax int16 // left channel maximum amplitude
	Rmax int16 // right channel maximum amplitude
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

func NewWaveform(samp []int16, rate uint) Waveform {
	left, right := WaveRanges(samp)
	return Waveform{samp, rate, max(left.max, -left.min), max(right.max, -right.min)}
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