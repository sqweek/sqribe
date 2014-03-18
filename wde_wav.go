package main

import (
	"github.com/skelterjohn/go.wde"
	"image/color"
	"image/draw"
	"image"
	"math/big"
	"math"
	"time"
	"fmt"
)

type changeMask int

const (
	WAV changeMask = 1 << iota + 1
	SCALE
	CURSOR
)

type WaveWidget struct {
	wav *Waveform
	score *Score
	first_frame FrameN
	frames_per_pixel int
	selection struct {
		min FrameN
		max FrameN
	}
	renderstate struct {
		rect image.Rectangle
		img *image.RGBA
		waveform *image.RGBA
		changed changeMask
	}
	cursor image.Point
	refresh chan image.Rectangle
	iolisten <-chan *Chunk
}

func NewWaveWidget(refresh chan image.Rectangle) *WaveWidget {
	var ww WaveWidget
	ww.first_frame = 0
	ww.frames_per_pixel = 512
	ww.renderstate.rect = image.Rect(0,0,0,0)
	ww.renderstate.img = nil
	ww.renderstate.changed = WAV
	ww.refresh = refresh
	return &ww
}

func (ww *WaveWidget) Rect() image.Rectangle {
	return ww.renderstate.rect
}

func (ww *WaveWidget) SelectAudioByTime(start, end time.Duration) {
	if ww.wav == nil {
		return
	}
	ww.selection.min = ww.wav.FrameAtTime(start)
	ww.selection.max = ww.wav.FrameAtTime(end)
	ww.renderstate.changed |= WAV // XXX doesn't really need to redraw waveform
	ww.refresh <- ww.renderstate.rect
}

func (ww *WaveWidget) SelectAudioSnapToBeats(start, end time.Duration) {
	if ww.wav == nil || ww.score == nil {
		return
	}
	ww.selection.min = ww.score.NearestBeat(ww.wav.FrameAtTime(start))
	ww.selection.max = ww.score.NearestBeat(ww.wav.FrameAtTime(end))
	// XXX could avoid redrawing waveform if selection rendered differently
	ww.renderstate.changed |= WAV
	ww.refresh <- ww.renderstate.rect
}

func (ww *WaveWidget) GetSelectedFrameRange() (FrameN, FrameN) {
	return ww.selection.min, ww.selection.max
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
				f0, fN := ww.VisibleFrameRange()
				s0, sN := ww.wav.SampleRange(f0, fN)
				if chunk.Intersects(s0, sN) {
					ww.renderstate.changed |= WAV
					ww.refresh <- ww.renderstate.rect
				}
			}
		}()
	}
	ww.renderstate.changed |= WAV
	ww.refresh <- ww.renderstate.rect
}

func (ww *WaveWidget) SetScore(score *Score) {
	ww.score = score
	//TODO listener stuff
}

func (ww *WaveWidget) VisibleFrameRange() (FrameN, FrameN) {
	w0 := ww.first_frame
	wN := w0 + FrameN(ww.frames_per_pixel) * FrameN(ww.renderstate.rect.Dx())
	return w0, wN
}

func (ww *WaveWidget) SetCursorByFrame(frame FrameN) {
	ww.cursor = image.Point{int(frame - ww.first_frame) / ww.frames_per_pixel, 0}
	ww.renderstate.changed |= CURSOR
	ww.refresh <- ww.renderstate.rect
}

func withinGrabDistance(x int, mouse image.Point) bool {
	dx := mouse.X - x
	return dx >= -2 && dx <= 2
}

func (ww *WaveWidget) FrameAtCursor() FrameN {
	cur0 := ww.cursor.Sub(ww.renderstate.rect.Min)
	return ww.FrameAtPixel(cur0.X)
}

func (ww *WaveWidget) FrameAtPixel(dx int) FrameN {
	return ww.first_frame + FrameN(dx * ww.frames_per_pixel)
}

func (ww *WaveWidget) PixelAtFrame(frame FrameN) int {
	return int(frame - ww.first_frame) / ww.frames_per_pixel
}

func (ww *WaveWidget) dragFn(min, max FrameN, ptr *FrameN, cm changeMask) DragFn {
	return func(pos image.Point) bool {
		f := ww.FrameAtPixel(pos.X - ww.renderstate.rect.Min.X)
		if f <= min || f >= max {
			return false
		}
		*ptr = f
		ww.renderstate.changed |= cm
		ww.refresh <- ww.renderstate.rect
		return true
	}
}

func (ww *WaveWidget) CursorIconAtPixel(mouse image.Point) (DragFn, Cursor) {
	nframes := ww.wav.ToFrame(ww.wav.NSamples)
	if withinGrabDistance(ww.PixelAtFrame(ww.selection.min), mouse) {
		return ww.dragFn(0, ww.selection.max, &ww.selection.min, WAV), ResizeLCursor
	}
	if withinGrabDistance(ww.PixelAtFrame(ww.selection.max), mouse) {
		return ww.dragFn(ww.selection.min, nframes, &ww.selection.max, WAV), ResizeRCursor
	}
	// TODO ignore beat grabs when sufficiently zoomed out
	lastFrame := ww.first_frame + FrameN(ww.renderstate.rect.Dx() * ww.frames_per_pixel)
	min, max := FrameN(0), nframes
	for i, beat := range(ww.score.beats) {
		if beat < ww.first_frame {
			min = 0
			continue
		} else if beat > lastFrame {
			break
		}
		if withinGrabDistance(ww.PixelAtFrame(beat), mouse) {
			if i + 1 < len(ww.score.beats) {
				max = ww.score.beats[i + 1]
			}
			return ww.dragFn(min, max, &ww.score.beats[i], SCALE), ResizeHCursor
		}
	}
	origin := mouse
	return func(pos image.Point)bool {
		r := ww.renderstate.rect
		minT := ww.TimeAtCursor(pos.X)
		maxT := ww.TimeAtCursor(origin.X)
		if maxT < minT {
			minT, maxT = maxT, minT
		}
		if origin.Y - r.Min.Y < r.Dy() / 5 {
			ww.SelectAudioByTime(minT, maxT)
		} else {
			ww.SelectAudioSnapToBeats(minT, maxT)
		}
		return true
	}, NormalCursor
}

func (ww *WaveWidget) SetCursorByPixel(mousePos image.Point) {
	ww.cursor = mousePos
	ww.renderstate.changed |= CURSOR
	ww.refresh <- ww.renderstate.rect
}

func (ww *WaveWidget) Scroll(amount float64) int {
	if ww.renderstate.rect.Empty() || ww.wav == nil {
		return 0
	}
	original := ww.first_frame
	width := ww.renderstate.rect.Size().X
	shift := FrameN((float64(width) * amount) * float64(ww.frames_per_pixel))
	rbound := ww.wav.ToFrame(ww.wav.NSamples) - FrameN((width + 1) * ww.frames_per_pixel)
	ww.first_frame += shift
	//fmt.Println(ww.wav.NSamples, width, ww.frames_per_pixel, ww.first_frame, rbound)
	if ww.first_frame < 0 || rbound < 0 {
		ww.first_frame = 0
	} else if ww.first_frame > rbound {
		ww.first_frame = rbound
	}
	diff := int(ww.first_frame - original)
	if diff != 0 {
		ww.renderstate.changed |= WAV
		ww.refresh <- ww.renderstate.rect
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
		ww.renderstate.changed |= WAV
		ww.refresh <- ww.renderstate.rect
	}
	return delta
}

// dst.Bounds() is the entire window, r is the area this widget is responsible for
func (ww *WaveWidget) Draw(dst wde.Image, r image.Rectangle) {
	change := ww.renderstate.changed
	ww.renderstate.changed = 0
	if !r.Eq(ww.renderstate.rect) {
		/* our widget size has chaged, redraw everything */
		change |= WAV | SCALE | CURSOR
	}
	if change != 0 {
		ww.renderstate.rect = r
		r0 := image.Rect(0, 0, r.Dx(), r.Dy())
		ww.renderstate.img = image.NewRGBA(r0)
		if ww.wav != nil {
			if change & WAV != 0 || ww.renderstate.waveform == nil {
				ww.renderstate.waveform = image.NewRGBA(r0)
				ww.drawWave(ww.renderstate.waveform, r0)
			}
			draw.Draw(ww.renderstate.img, r0, ww.renderstate.waveform, image.ZP, draw.Src)
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

func scale(chMin, chMax int16, yscale float64) (int, int) {
	var min, max int
	if chMin < 0 {
		min = int(float64(chMin) / yscale)
	}
	if chMax > 0 {
		max = int(float64(chMax) / yscale)
	}
	return min, max
}

func (ww *WaveWidget) drawWave(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xee, 0xee, 0xcc, 255}
	cl := color.RGBA{0x99, 0x99, 0xcc, 255}
	ci := color.RGBA{0xbb, 0x99, 0xbb, 255}
	cr := color.RGBA{0xbb, 0x99, 0x99, 255}
	csel := color.RGBA{0xdd, 0xdd, 0xdd, 255}
	f0 := ww.first_frame
	fpp := FrameN(ww.frames_per_pixel)
	sel0, selN := ww.GetSelectedFrameRange()
	selR := image.Rect(int((sel0 - f0)/fpp), r.Min.Y, int((selN - f0)/fpp), r.Max.Y)
	yorigin := (r.Min.Y + r.Max.Y) / 2
	size := r.Size()
	yscale := (float64(ww.wav.MaxAmp()) / float64(size.Y / 2))
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	draw.Draw(dst, selR, &image.Uniform{csel}, image.ZP, draw.Src)
	chunks := ww.wav.GetFrames(f0, f0 + FrameN(size.X) * fpp)
	for dx := 0; dx < size.X; dx++ {
		pixS0, pixSN := ww.wav.SampleRange(f0 + fpp * FrameN(dx), f0 + fpp * FrameN(dx+1))
		pixSamples := Extract(chunks, pixS0, pixSN)
		ext := ww.wav.ChannelExtents(pixSamples)
		/* FIXME remove two channel assumption */
		lmin, lmax := scale(ext[0], ext[1], yscale)
		rmin, rmax := scale(ext[2], ext[3], yscale)
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

type NoteHead struct {
	col color.RGBA
	p image.Point
	r int
	α float64
}

func (n *NoteHead) ColorModel() color.Model {
	return color.RGBAModel
}

func (n *NoteHead) Bounds() image.Rectangle {
	return image.Rect(n.p.X - n.r, n.p.Y - n.r, n.p.X + n.r + 1, n.p.Y + n.r + 1)
}

func (n *NoteHead) At(x, y int) color.Color {
	xx, yy, rr := float64(x - n.p.X)+0.5, float64(y - n.p.Y)+0.5, float64(n.r)
	rx := xx * math.Cos(n.α) - yy * math.Sin(n.α)
	ry := xx * math.Sin(n.α) + yy * math.Cos(n.α)
	if rx*rx + 1.25*1.25*ry*ry < rr*rr {
		return n.col
	}
	return color.RGBA{0, 0, 0, 0}
}

func (ww *WaveWidget) drawScale(dst draw.Image, r image.Rectangle) {
	if ww.score == nil {
		return
	}
	black4 := color.RGBA{0x00, 0x00, 0x00, 0x88}
	black1 := color.RGBA{0x00, 0x00, 0x00, 0x22}
	lastFrame := ww.first_frame + FrameN(r.Dx() * ww.frames_per_pixel)
	minX, maxX := -1, -1
	for i, beat := range(ww.score.beats) {
		if beat < ww.first_frame {
			minX = r.Min.X
			continue
		}
		if beat > lastFrame {
			maxX = r.Max.X
			break
		}
		x := int(beat - ww.first_frame) / ww.frames_per_pixel
		if minX == -1 {
			minX = x
		}
		maxX = x
		line := image.Rect(x, 0, x+1, r.Max.Y)
		black := black1
		if i % 4 == 0 {
			black = black4
		}
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Over)
	}
	if minX >= maxX {
		return
	}
	yspacing := 10
	mid := (r.Min.Y + r.Max.Y) / 2
	minY, maxY := mid - 2 * yspacing, mid + 2 * yspacing
	for y := minY; y <= maxY; y += yspacing {
		line := image.Rect(minX, y, maxX, y+1)
		draw.Draw(dst, line, &image.Uniform{black4}, image.ZP, draw.Over)
	}

	ww.drawProspectiveNote(dst, r, mid, yspacing)
}

func colourFor(offset *big.Rat) color.RGBA {
	α := uint8(0xff)
	switch (offset.RatString()) {
	case "1": fallthrough
	case "0": fallthrough
	case "1/8": return color.RGBA{0xff, 0x00, 0x00, α}

	case "1/16": fallthrough
	case "3/16": return color.RGBA{0x00, 0x00, 0xff, α}

	case "1/32": fallthrough
	case "3/32": fallthrough
	case "5/32": fallthrough
	case "7/32": return color.RGBA{0xff, 0xff, 0x00, α}

	case "1/24": fallthrough
	case "3/24": fallthrough
	case "5/24": return color.RGBA{0x88, 0x00, 0x88, α}

	case "1/12": fallthrough
	case "1/6": return color.RGBA{0xff, 0x00, 0xff, α}
	}
	return color.RGBA{0x00, 0x00, 0x00, α}
}

func (ww *WaveWidget) drawProspectiveNote(dst draw.Image, r image.Rectangle, mid, yspacing int) {
	black2 := color.RGBA{0x00, 0x00, 0x00, 0x44}
	cur0 := ww.cursor.Sub(ww.renderstate.rect.Min)
	noteY := snapto(cur0.Y, mid, yspacing / 2)

	framec := ww.first_frame + FrameN(cur0.X * ww.frames_per_pixel)
	beatf, ok := ww.score.ToBeat(framec)
	if !ok {
		return
	}
	beati, offset := ww.score.Quantize(beatf)
	if beati + 1 >= len(ww.score.beats) {
		return
	}

	beat0, beat1 := ww.score.beats[beati], ww.score.beats[beati+1]
//for beatf = float64(beati); beatf <= float64(beati + 1); beatf += 1.0/256.0 {
//	_, offset = ww.score.Quantize(beatf)
	α, _ := offset.Float64()
	α *= float64(ww.score.beatLen.Denom().Int64())
	frame := FrameN((1 - α) * float64(beat0) + α * float64(beat1))
	noteX := ww.PixelAtFrame(frame)
//	l := image.Rect(noteX, mid - 100, noteX + 1, mid + 101)
//	draw.Draw(dst, l, &image.Uniform{colourFor(offset)}, image.ZP, draw.Src)
//}
//	noteX := cur0.X

	/* ledger lines */
	ydist := int(math.Abs(float64(noteY - mid)))
	sgn := 1
	if (mid > noteY) {
		sgn = -1
	}
	for dy := yspacing * 3; dy <= ydist; dy += yspacing {
		width := yspacing / 2 + 1
		line := image.Rect(noteX - width, mid + sgn*dy, noteX + width + 1, mid + sgn*(dy + 1))
		draw.Draw(dst, line, &image.Uniform{black2}, image.ZP, draw.Over)
	}

	draw.Draw(dst, dst.Bounds(), &NoteHead{colourFor(offset), image.Point{noteX, noteY}, yspacing/2, 35.0}, image.ZP, draw.Over)
}

func snapto(x, origin, step int) int {
	d := x - origin
	var sgn int
	if (d < 0) {
		sgn = -1
	} else {
		sgn = 1
	}
	rem := (sgn * d) % step
	if rem < step/2 {
		return x - sgn * rem
	}
	return x + sgn * (step - rem)
}

func (ww *WaveWidget) TimeAtCursor(dx int) time.Duration {
	if ww.wav == nil {
		return 0.0
	}
	frame := ww.first_frame + FrameN(dx*ww.frames_per_pixel)
	return ww.wav.TimeAtFrame(frame)
}

func (ww *WaveWidget) Status() string {
	cur0 := ww.cursor.Sub(ww.renderstate.rect.Min)
	framec := ww.first_frame + FrameN(cur0.X * ww.frames_per_pixel)
	beatf, ok := ww.score.ToBeat(framec)
	var offset *big.Rat
	if !ok {
		offset = big.NewRat(0, 1)
	}
	_, offset = ww.score.Quantize(beatf)

	return fmt.Sprintf("s0=%d spp=%d pos=%v", ww.first_frame, ww.frames_per_pixel, offset)
}
