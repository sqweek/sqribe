package main

import (
	"github.com/skelterjohn/go.wde"
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
	first_frame int64
	frames_per_pixel int
	selection struct {
		min time.Duration
		max time.Duration
	}
	renderstate struct {
		rect image.Rectangle
		img *image.RGBA
		modelChanged bool
	}
	cursor image.Point
	refresh chan image.Rectangle
	iolisten <-chan *Chunk
}

func NewWaveWidget(refresh chan image.Rectangle) *WaveWidget {
	var ww WaveWidget
	ww.wav = nil
	ww.bpm = 120
	ww.first_frame = 0
	ww.frames_per_pixel = 512
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
	fmt.Println("selected", ww.selection.min, ww.selection.max, ww.FrameAtTime(ww.selection.min), ww.FrameAtTime(ww.selection.max))
	ww.renderstate.modelChanged = true
}

func (ww *WaveWidget) GetSelectedFrameRange() (int64, int64) {
	return ww.FrameAtTime(ww.selection.min), ww.FrameAtTime(ww.selection.max)	
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
				w0, wN := ww.VisibleFrameRange()
				s0, sN := w0*2, wN*2 + (2 - 1)
				if chunk.Intersects(uint64(s0), uint64(sN)) {
					ww.renderstate.modelChanged = true
					ww.refresh <- image.Rect(0, 0, 0, 0)
				}
			}
		}()
	}
	ww.renderstate.modelChanged = true
	ww.refresh <- image.Rect(0, 0, 0, 0)
}

func (ww *WaveWidget) VisibleFrameRange() (int64, int64) {
	w0 := ww.first_frame
	wN := w0 + int64(ww.frames_per_pixel) * int64(ww.renderstate.rect.Dx())
	return w0, wN
}

func (ww *WaveWidget) SetCursorBySample(sample int64) {
	frame := sample / 2
	ww.cursor = image.Point{int(frame - ww.first_frame) / ww.frames_per_pixel, 0}
	ww.renderstate.modelChanged = true 
}

func (ww *WaveWidget) SetCursorByPixel(mousePos image.Point) {
	ww.cursor = mousePos
	ww.renderstate.modelChanged = true
}

func (ww *WaveWidget) Scroll(amount float64) int {
	if ww.renderstate.rect.Empty() || ww.wav == nil {
		return 0
	}
	original := ww.first_frame
	width := ww.renderstate.rect.Size().X
	shift := int64((float64(width) * amount) * float64(ww.frames_per_pixel))
	rbound := int64(ww.wav.NSamples/2) - int64((width + 1) * ww.frames_per_pixel)
	ww.first_frame += shift
	//fmt.Println(ww.wav.NSamples, width, ww.frames_per_pixel, ww.first_frame, rbound)
	if ww.first_frame < 0 || rbound < 0 {
		ww.first_frame = 0
	} else if ww.first_frame > rbound {
		ww.first_frame = rbound
	}
	diff := int(ww.first_frame - original)
	if diff != 0 {
		ww.renderstate.modelChanged = true
	}
	return diff
}

func (ww *WaveWidget) Zoom(factor float64) float64 {
	original := float64(ww.frames_per_pixel)
	ww.frames_per_pixel = int(original * factor)
	if ww.frames_per_pixel < 1 {
		ww.frames_per_pixel = 1
	}
	delta := float64(ww.frames_per_pixel) / original
	if delta != 1.0 {
		ww.renderstate.modelChanged = true
	}
	return delta
}

// dst.Bounds() is the entire window, r is the area this widget is responsible for
func (ww *WaveWidget) Draw(dst wde.Image, r image.Rectangle) {
	if ww.renderstate.modelChanged || !r.Eq(ww.renderstate.rect) {
		ww.renderstate.rect = r
		r0 := image.Rect(0, 0, r.Dx(), r.Dy())
		ww.renderstate.modelChanged = false
		ww.renderstate.img = image.NewRGBA(r0)
		if ww.wav != nil {
			ww.drawWave(ww.renderstate.img, r0)
		}
		ww.drawScale(ww.renderstate.img, r0)

		curcol := color.RGBA{0, 0xdd, 0, 255}
		draw.Draw(ww.renderstate.img, image.Rect(ww.cursor.X, 0, ww.cursor.X+1, r.Dy()), &image.Uniform{curcol}, image.ZP, draw.Src)
		dst.CopyRGBA(ww.renderstate.img, r)
		//draw.Draw(dst, r, ww.renderstate.img, r.Min, draw.Src)
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
	f0 := ww.first_frame
	fpp := int64(ww.frames_per_pixel)
	sel0, selN := ww.GetSelectedFrameRange()
	selR := image.Rect(int((sel0 - f0)/fpp), r.Min.Y, int((selN - f0)/fpp), r.Max.Y)
	yorigin := (r.Min.Y + r.Max.Y) / 2
	size := r.Size()
	yscale := (float64(ww.wav.Max()) / float64(size.Y / 2))
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	draw.Draw(dst, selR, &image.Uniform{csel}, image.ZP, draw.Src)
	chunks := ww.wav.GetFrames(f0, f0 + int64(size.X) * fpp)
	s0 := 2 * f0
	for dx := 0; dx < size.X; dx++ {
		pixS0 := uint64(s0 + fpp * int64(dx) * 2)
		pixSN := uint64(s0 + fpp * int64(dx+1) * 2 - 1)
		pixSamples := Extract(chunks, pixS0, pixSN)
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
	framesPerBar := int64(secondsPerBar * float64(ww.wav.rate))
	anchorFrame := ww.FrameAtTime(ww.anchor)
	for anchorFrame > ww.first_frame + framesPerBar {
		anchorFrame -= framesPerBar
	}
	for anchorFrame < ww.first_frame {
		anchorFrame += framesPerBar
	}
	lastFrame := ww.first_frame + int64(r.Dx() * ww.frames_per_pixel)
	yspacing := 10
	mid := (r.Min.Y + r.Max.Y) / 2
	for i := -2; i <= 2; i++ {
		y := mid + i * yspacing
		line := image.Rect(r.Min.X, y, r.Max.X, y+1)
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Src)
	}
	for f := anchorFrame; f < lastFrame; f += framesPerBar {
		x := int(f - ww.first_frame) / ww.frames_per_pixel
		line := image.Rect(x, mid - 2*yspacing, x+1, mid + 2*yspacing + 1)
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Src)
	}
}

func (ww *WaveWidget) TimeAtCursor(dx int) time.Duration {
	if ww.wav == nil {
		return 0.0
	}
	frame := ww.first_frame + int64(dx*ww.frames_per_pixel)
	return ww.wav.TimeAtFrame(frame)
}

func (ww *WaveWidget) FrameAtTime(t time.Duration) int64 {
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
	return fmt.Sprintf("s0=%d spp=%d", ww.first_frame, ww.frames_per_pixel)
}
