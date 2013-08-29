package main

import (
	_ "github.com/skelterjohn/go.wde"
	"image/color"
	"image/draw"
	"image"
	"math"
	//"log"
	"fmt"
)

type WaveWidget struct {
	wav *Waveform
	first_sample int
	samples_per_pixel int
	renderstate struct {
		rect image.Rectangle
		img draw.Image
		modelChanged bool
	}
}

func NewWaveWidget() *WaveWidget {
	var ww WaveWidget
	ww.wav = nil
	ww.first_sample = 0
	ww.samples_per_pixel = 512
	ww.renderstate.rect = image.Rect(0,0,0,0)
	ww.renderstate.img = nil
	ww.renderstate.modelChanged = true
	return &ww
}

func (ww *WaveWidget) SetWaveform(wav *Waveform) {
	ww.wav = wav
	/* TODO paint */
}

func (ww *WaveWidget) Scroll(amount float64) int {
	if ww.renderstate.rect.Empty() || ww.wav == nil {
		return 0
	}
	original := ww.first_sample
	width := ww.renderstate.rect.Size().X
	shift := int((float64(width) * amount) * float64(ww.samples_per_pixel))
	rbound := len(ww.wav.Samples)/2 - (width + 1) * ww.samples_per_pixel
	ww.first_sample += shift
	//log.Println(len(ww.wav.Samples), width, ww.samples_per_pixel, ww.first_sample, rbound)
	if ww.first_sample < 0 || rbound < 0 {
		ww.first_sample = 0
	} else if ww.first_sample > rbound {
		ww.first_sample = rbound
	}
	diff := ww.first_sample - original
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
		ww.renderstate.rect = dst.Bounds()
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
	s0 := ww.first_sample
	spp := ww.samples_per_pixel
	yorigin := (r.Min.Y + r.Max.Y) / 2
	size := r.Size()
	yscale := (float64(ww.wav.Max()) / float64(size.Y / 2))
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	for dx := 0; dx < size.X; dx++ {
		i0 := 2*(dx*spp + s0)
		iN := 2*((dx+1)*spp + s0)
		if i0 > len(ww.wav.Samples) {
			return
		}
		if iN > len(ww.wav.Samples) {
			iN = len(ww.wav.Samples)
		}
		pixSamples := ww.wav.Samples[i0:iN]
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
	spacing := 10
	mid := (r.Min.Y + r.Max.Y) / 2
	for i := -2; i <= 2; i++ {
		y := mid + i * spacing
		line := image.Rect(r.Min.X, y, r.Max.X, y+1)
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Src)
	}
}

func (ww *WaveWidget) SampleAt(dx int) int {
	return ww.first_sample + dx*ww.samples_per_pixel
}

func (ww *WaveWidget) Status() string {
	return fmt.Sprintf("s0=%d spp=%d", ww.first_sample, ww.samples_per_pixel)
}
