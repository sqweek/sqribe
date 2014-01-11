package main

import (
	_ "github.com/skelterjohn/go.wde"
	"image/color"
	"image/draw"
	"image"
	"math"
	"time"
	"fmt"
)

type WaveWidget struct {
	wav *Waveform
	bpm float64
	anchor time.Duration
	first_sample int64
	samples_per_pixel int
	selection struct {
		min time.Duration
		max time.Duration
	}
	renderstate struct {
		rect image.Rectangle
		img draw.Image
		modelChanged bool
	}
	refresh chan image.Rectangle
	iolisten <-chan *Chunk
}

func NewWaveWidget(refresh chan image.Rectangle) *WaveWidget {
	var ww WaveWidget
	ww.wav = nil
	ww.bpm = 120
	ww.first_sample = 0
	ww.samples_per_pixel = 512
	ww.renderstate.rect = image.Rect(0,0,0,0)
	ww.renderstate.img = nil
	ww.renderstate.modelChanged = true
	ww.refresh = refresh
	return &ww
}

func (ww *WaveWidget) Rect() image.Rectangle {
	return ww.renderstate.rect
}

func (ww *WaveWidget) SetBeatAnchor(anchor time.Duration) {
	ww.anchor = anchor
	ww.renderstate.modelChanged = true
}

func (ww *WaveWidget) SetBpm(bpm float64) {
	ww.bpm = bpm
	ww.renderstate.modelChanged = true
}

func (ww *WaveWidget) SelectAudioByTime(start, end time.Duration) {
	ww.selection.min = start
	ww.selection.max = end
	ww.renderstate.modelChanged = true
}

func nearestBar(x, anchor, barDuration time.Duration) time.Duration {
	rem := (x - anchor) % barDuration
	if rem > barDuration/2 {
		return x - rem + barDuration
	}
	return x - rem
}

func (ww *WaveWidget) SelectAudioSnapToBars(start, end time.Duration) {
	beatsPerBar := 4.0
	barDuration := time.Microsecond * time.Duration(1000000 * beatsPerBar / (float64(ww.bpm) / 60.0))
	ww.selection.min = nearestBar(start, ww.anchor, barDuration)
	ww.selection.max = nearestBar(end, ww.anchor, barDuration)
	ww.renderstate.modelChanged = true
}

func (ww *WaveWidget) GetSelectedSampleRange() (int64, int64) {
	return ww.SampleAtTime(ww.selection.min), ww.SampleAtTime(ww.selection.max)	
}

func (ww *WaveWidget) SetWaveform(wav *Waveform) {
	if ww.wav != nil {
		ww.wav.cache.ignore(ww.iolisten)
	}
	ww.wav = wav
	if ww.wav != nil {
		iolisten := ww.wav.cache.listen()
		ww.iolisten = iolisten
		go func() {
			for {
				chunk, ok := <-iolisten
				if !ok {
					return
				}
				c0, cN := int64(chunk.I0)/2, (int64(chunk.I0) + int64(len(chunk.Data)))/2
				w0, wN := ww.SampleRange()
				fmt.Printf("wav heard about chunk %d i/o (%d - %d)  visible (%d - %d)\n", chunk.id, c0, cN, w0, wN)
				if (c0 >= w0 && c0 <= wN) || (cN >= w0 && cN <= wN) {
					ww.renderstate.modelChanged = true
					ww.refresh <- image.Rect(0, 0, 0, 0)
				}
			}
		}()
	}
	ww.renderstate.modelChanged = true
	ww.refresh <- image.Rect(0, 0, 0, 0)
}

func (ww *WaveWidget) SampleRange() (int64, int64) {
	w0 := ww.first_sample
	wN := w0 + int64(ww.samples_per_pixel) * int64(ww.renderstate.rect.Dx())
	return w0, wN
}

func (ww *WaveWidget) Scroll(amount float64) int {
	if ww.renderstate.rect.Empty() || ww.wav == nil {
		return 0
	}
	original := ww.first_sample
	width := ww.renderstate.rect.Size().X
	shift := int64((float64(width) * amount) * float64(ww.samples_per_pixel))
	rbound := int64(ww.wav.NSamples/2) - int64((width + 1) * ww.samples_per_pixel)
	ww.first_sample += shift
	//fmt.Println(ww.wav.NSamples, width, ww.samples_per_pixel, ww.first_sample, rbound)
	if ww.first_sample < 0 || rbound < 0 {
		ww.first_sample = 0
	} else if ww.first_sample > rbound {
		ww.first_sample = rbound
	}
	diff := int(ww.first_sample - original)
	if diff != 0 {
		ww.renderstate.modelChanged = true
	}
	return diff
}

func (ww *WaveWidget) Zoom(factor float64) float64 {
	original := float64(ww.samples_per_pixel)
	ww.samples_per_pixel = int(original * factor)
	if ww.samples_per_pixel < 1 {
		ww.samples_per_pixel = 1
	}
	delta := float64(ww.samples_per_pixel) / original
	if delta != 1.0 {
		ww.renderstate.modelChanged = true
	}
	return delta
}

func (ww *WaveWidget) Draw(dst draw.Image, r image.Rectangle) {
	if ww.renderstate.modelChanged || !dst.Bounds().Eq(ww.renderstate.rect) {
		ww.renderstate.rect = r
		ww.renderstate.modelChanged = false
		ww.renderstate.img = image.NewRGBA(r)
		if ww.wav != nil {
			ww.drawWave(ww.renderstate.img, r)
		}
		ww.drawScale(ww.renderstate.img, r)
		draw.Draw(dst, r, ww.renderstate.img, r.Min, draw.Src)
	}
}

func slog(s int16) float64 {
	return float64(s)
	if s == 0 {
		return 0.0
	} else if s < 0 {
		return -math.Log(float64(-s))
	} else {
		return math.Log(float64(s))
	}
}

func (ww *WaveWidget) drawWave(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xee, 0xee, 0xcc, 255}
	cl := color.RGBA{0x99, 0x99, 0xcc, 255}
	ci := color.RGBA{0xbb, 0x99, 0xbb, 255}
	cr := color.RGBA{0xbb, 0x99, 0x99, 255}
	csel := color.RGBA{0xdd, 0xdd, 0xdd, 255}
	s0 := ww.first_sample
	spp := int64(ww.samples_per_pixel)
	sel0, selN := ww.GetSelectedSampleRange()
	selR := image.Rect(int((sel0 - s0)/spp), r.Min.Y, int((selN - s0)/spp), r.Max.Y)
	yorigin := (r.Min.Y + r.Max.Y) / 2
	size := r.Size()
	yscale := (float64(ww.wav.Max()) / float64(size.Y / 2))
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	draw.Draw(dst, selR, &image.Uniform{csel}, image.ZP, draw.Src)
	for dx := 0; dx < size.X; dx++ {
		i0 := 2*uint64(int64(dx)*spp + s0)
		iN := 2*uint64(int64(dx+1)*spp + s0)
		if i0 > ww.wav.NSamples {
			return
		}
		if iN > ww.wav.NSamples {
			iN = ww.wav.NSamples
		}
		pixSamples := ww.wav.GetSamples(i0, iN)
		left, right := WaveRanges(pixSamples)
		var lmin, lmax, rmin, rmax int
		if left.min > 0 {
			lmin = 0
		} else {
			lmin = int(float64(left.min) / yscale)
		}
		if left.max < 0 {
			lmax = 0
		} else {
			lmax = int(float64(left.max) / yscale)
		}
		if right.min > 0 {
			rmin = 0
		} else {
			rmin = int(float64(right.min) / yscale)
		}
		if right.max < 0 {
			rmax = 0
		} else {
			rmax = int(float64(right.max) / yscale)
		}
		x := r.Min.X + dx
		rl := image.Rect(x, yorigin - lmax, x + 1, yorigin - lmin + 1)
		rr := image.Rect(x, yorigin - rmax, x + 1, yorigin - rmin + 1)
		ri := rl.Intersect(rr)
		draw.Draw(dst, rl, &image.Uniform{cl}, image.ZP, draw.Src)
		draw.Draw(dst, rr, &image.Uniform{cr}, image.ZP, draw.Src)
		if !ri.Empty() {
			draw.Draw(dst, ri, &image.Uniform{ci}, image.ZP, draw.Src)
		}
	}
}

func (ww *WaveWidget) drawScale(dst draw.Image, r image.Rectangle) {
	black := color.RGBA{0x00, 0x00, 0x00, 0xff}
	beatsPerBar := 4.0
	secondsPerBar := beatsPerBar / (float64(ww.bpm) / 60.0)
	barWidth := int(secondsPerBar * float64(ww.wav.rate) / float64(ww.samples_per_pixel))
	anchor := ww.SampleAtTime(ww.anchor)
	anchorPixel := int((anchor - ww.first_sample) / int64(ww.samples_per_pixel))
	for anchorPixel > r.Min.X + barWidth {
		anchorPixel -= barWidth
	}
	for anchorPixel < r.Min.X {
		anchorPixel += barWidth
	}
	yspacing := 10
	mid := (r.Min.Y + r.Max.Y) / 2
	for i := -2; i <= 2; i++ {
		y := mid + i * yspacing
		line := image.Rect(r.Min.X, y, r.Max.X, y+1)
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Src)
	}
	for x := anchorPixel; x < r.Max.X; x += barWidth {
		line := image.Rect(x, mid - 2*yspacing, x+1, mid + 2*yspacing + 1)
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Src)
	}
}

func (ww *WaveWidget) TimeAtCursor(dx int) time.Duration {
	if ww.wav == nil {
		return 0.0
	}
	sample := ww.first_sample + int64(dx*ww.samples_per_pixel)
	durPerSample := time.Second / time.Duration(ww.wav.rate)
	return time.Duration(sample) * durPerSample
}

func (ww *WaveWidget) SampleAtTime(t time.Duration) int64 {
	if ww.wav == nil {
		return 0
	}
	s := int64(float64(t) / float64(time.Second) * float64(ww.wav.rate))
	if s < 0 {
		s = 0
	}
	if s >= int64(ww.wav.NSamples/2) {
		s = int64(ww.wav.NSamples/2 - 1)
	}
	return s
}

func (ww *WaveWidget) SixtyFourthAtTime(t time.Duration) int {
	bps := float64(ww.bpm) / 60.0
	return int(float64(t) / float64(time.Second) * 16.0 * bps)
}

func (ww *WaveWidget) Status() string {
	return fmt.Sprintf("s0=%d spp=%d", ww.first_sample, ww.samples_per_pixel)
}
