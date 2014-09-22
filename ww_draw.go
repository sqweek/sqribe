package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/big"
	"time"
)

// dst.Bounds() is the entire window, r is the area this widget is responsible for
func (ww *WaveWidget) Draw(dst draw.Image, r image.Rectangle) {
	change := ww.renderstate.changed
	ww.renderstate.changed = 0
	if !r.Eq(ww.rect.r) {
		/* our widget size has chaged, redraw everything */
		change |= WAV | SCALE | CURSOR
	}
	if change != 0 {
		axish := 20
		ww.rect.r = r
		ww.rect.wave = image.Rect(r.Min.X, r.Min.Y + axish, r.Max.X, r.Max.Y - axish)
		ww.renderstate.img = image.NewRGBA(ww.rect.r)
		if ww.wav != nil {
			if change & WAV != 0 || ww.renderstate.waveform == nil {
				ww.renderstate.waveform = image.NewRGBA(ww.rect.wave)
				ww.drawWave(ww.renderstate.waveform, ww.rect.wave)
			}
			wMin := image.Point{r.Min.X, r.Min.Y + axish}
			draw.Draw(ww.renderstate.img, ww.rect.wave, ww.renderstate.waveform, wMin, draw.Src)
		}
		ww.drawScale(ww.renderstate.img, ww.rect.wave)

		curcol := color.RGBA{0, 0xdd, 0, 255}
		draw.Draw(ww.renderstate.img, image.Rect(ww.cursorX, r.Min.Y, ww.cursorX+1, r.Max.Y), &image.Uniform{curcol}, r.Min, draw.Src)
		ww.drawBeatAxis(ww.renderstate.img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y + axish))
		ww.drawTimeAxis(ww.renderstate.img, image.Rect(r.Min.X, r.Max.Y - axish, r.Max.X, r.Max.Y))
	}
	draw.Draw(dst, r, ww.renderstate.img, r.Min, draw.Src)
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

func colourFor(offset *big.Rat) color.RGBA {
	α := uint8(0xff)
	switch (offset.RatString()) {
	case "1": fallthrough
	case "0": fallthrough
	case "1/2": return color.RGBA{0xff, 0x00, 0x00, α}

	case "1/4": fallthrough
	case "3/4": return color.RGBA{0x00, 0x00, 0xff, α}

	case "1/8": fallthrough
	case "3/8": fallthrough
	case "5/8": fallthrough
	case "7/8": return color.RGBA{0xff, 0xff, 0x00, α}

	// this is a bit too close to the blue right next door
	case "1/6": fallthrough
	case "3/6": fallthrough
	case "5/6": return color.RGBA{0x88, 0x00, 0x88, α}

	case "1/3": fallthrough
	case "4/6": return color.RGBA{0xff, 0x00, 0xff, α}
	}
	return color.RGBA{0x00, 0x00, 0x00, α}
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
	f0_get, dx0 := f0, 0
	if f0 < 0 {
		f0_get = 0
		dx0 = 1 + int(-f0 / fpp)
	}
	size := r.Size()
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	if dx0 >= size.X {
		return
	}
	sel0, selN := ww.GetSelectedFrameRange()
	selR := image.Rect(int((sel0 - f0)/fpp), r.Min.Y, int((selN - f0)/fpp), r.Max.Y)
	yorigin := (r.Min.Y + r.Max.Y) / 2
	yscale := (float64(ww.wav.MaxAmp()) / float64(size.Y / 2))
	draw.Draw(dst, selR, &image.Uniform{csel}, image.ZP, draw.Src)
	chunks := ww.wav.GetFrames(f0_get, f0 + FrameN(size.X) * fpp)
	for dx := dx0; dx < size.X; dx++ {
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

func (ww *WaveWidget) drawScale(dst draw.Image, r image.Rectangle) {
	if ww.score == nil {
		return
	}
	black4 := color.RGBA{0x00, 0x00, 0x00, 0x88}
	black1 := color.RGBA{0x00, 0x00, 0x00, 0x22}
	_, lastFrame := ww.VisibleFrameRange()
	minX, maxX := -1, -1
	for i, beat := range(ww.score.beats) {
		if beat.frame < ww.first_frame {
			minX = r.Min.X
			continue
		}
		if beat.frame > lastFrame {
			maxX = r.Max.X
			break
		}
		x := ww.PixelAtFrame(beat.frame)
		if minX == -1 {
			minX = x
		}
		maxX = x
		line := image.Rect(x, r.Min.Y, x+1, r.Max.Y)
		black := black1
		if i % 4 == 0 {
			black = black4
		}
		draw.Draw(dst, image.Rect(x-3, r.Min.Y, x+4, r.Min.Y+1), &image.Uniform{black}, r.Min, draw.Over)
		draw.Draw(dst, image.Rect(x-2, r.Min.Y+1, x+3, r.Min.Y+2), &image.Uniform{black}, r.Min, draw.Over)
		draw.Draw(dst, image.Rect(x-1, r.Min.Y+2, x+2, r.Min.Y+3), &image.Uniform{black}, r.Min, draw.Over)
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Over)
	}
	if minX >= maxX {
		return
	}
	mid := r.Min.Y + r.Dy() / 2
	minY, maxY := mid - 2 * yspacing, mid + 2 * yspacing
	for y := minY; y <= maxY; y += yspacing {
		line := image.Rect(minX, y, maxX, y+1)
		draw.Draw(dst, line, &image.Uniform{black4}, image.ZP, draw.Over)
	}

	ww.drawNotes(dst, r, mid)

	ww.drawProspectiveNote(dst, r, mid)
}

func (ww *WaveWidget) drawNote(dst draw.Image, r image.Rectangle, mid int, beatf float64, delta int, accidental *int, prospective bool) {
	var col, black color.RGBA
	if prospective {
		black = color.RGBA{0, 0, 0, 0x44}
		_, offset := ww.score.Quantize(beatf)
		col = colourFor(offset)
	} else {
		black = color.RGBA{0, 0, 0, 0xff}
		col = black
	}
	f0, fN := ww.VisibleFrameRange()
	frame, _ := ww.score.ToFrame(beatf)
	if frame < f0 || frame > fN {
		return
	}

	x := ww.PixelAtFrame(frame)
	y := mid - (yspacing / 2) * delta

	/* ledger lines */
	ydist := int(math.Abs(float64(y - mid)))
	sgn := 1
	if (mid > y) {
		sgn = -1
	}
	for dy := yspacing * 3; dy <= ydist; dy += yspacing {
		width := yspacing / 2 + 1
		line := image.Rect(x - width, mid + sgn*dy, x + width + 1, mid + sgn*(dy + 1))
		draw.Draw(dst, line, &image.Uniform{black}, image.ZP, draw.Over)
	}

	draw.Draw(dst, r, newNoteHead(col, image.Point{x, y}, yspacing/2, 35.0), r.Min, draw.Over)
	if accidental != nil {
		draw.Draw(dst, r, newAccidental(col, image.Point{x - yspacing, y}, yspacing/2, *accidental), r.Min, draw.Over)
	}
}

func (ww *WaveWidget) drawNotes(dst draw.Image, r image.Rectangle, mid int) {
	for _, note := range(ww.score.notes) {
		delta, accidental := ww.score.LineForPitch(note.Pitch)
		ww.drawNote(dst, r, mid, ww.score.Beatf(note), delta, accidental, false)
	}
}

func (ww *WaveWidget) drawProspectiveNote(dst draw.Image, r image.Rectangle, mid int) {
	s := ww.getMouseState(ww.mouse.pos)
	if s.note == nil {
		return
	}
	n := ww.mkNote(s.note)
	ww.drawNote(dst, r, mid, ww.score.Beatf(n), s.note.delta, nil, true)
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

func tickRect(r image.Rectangle, bottom bool, x, size int) image.Rectangle {
	if bottom {
		return image.Rect(x, r.Max.Y - size, x + 1, r.Max.Y)
	}
	return image.Rect(x, r.Min.Y, x + 1, r.Min.Y + size)
}

/* a0: axis value of first tick. aN: axis value of last tick. Δa: distance between major ticks */
func (ww *WaveWidget) drawTicks(dst draw.Image, r image.Rectangle, bottom bool, a0, aN, Δa float64, aToX func(float64)int, label func(float64)string) {
	targetPixPerTick := 30.0
	bg := color.RGBA{0x55, 0x44, 0x44, 0xff}
	fg := color.RGBA{0xcc, 0xcc, 0xbb, 0xff}
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Over)
	x0 := aToX(a0)
	xN := aToX(aN)
	pixPerMaj := float64(xN - x0) / (aN - a0)
	ΔMaj := Δa
	minPerMaj := 0
	if (pixPerMaj < targetPixPerTick) {
		ΔMaj = math.Trunc(0.5 + targetPixPerTick / pixPerMaj)
	} else {
		minPerMaj = int(pixPerMaj / targetPixPerTick) - 1
	}
	textSpacing := 50
	textY := r.Min.Y + 14
	if bottom {
		textY = r.Max.Y - 14
	}
	lastTextX := r.Min.X - textSpacing
	for a := a0; aToX(a) < xN + int(pixPerMaj); a += ΔMaj {
		for i := 1; i <= minPerMaj; i++ {
			am := a + float64(i) * (ΔMaj / float64(minPerMaj + 1))
			if am >= a0 && am <= aN {
				x := aToX(am)
				draw.Draw(dst, tickRect(r, bottom, x, 4), &image.Uniform{fg}, image.ZP, draw.Over)
			}
		}
		if a >= a0 && a <= aN {
			x := aToX(a)
			draw.Draw(dst, tickRect(r, bottom, x, 7), &image.Uniform{fg}, image.ZP, draw.Over)
			if x > lastTextX + textSpacing {
				lastTextX = x
				G.font.luxi.DrawC(dst, fg, r, label(a), image.Point{x, textY})
			}
		}
	}
}

func (ww *WaveWidget) drawBeatAxis(dst draw.Image, r image.Rectangle) {
	score := ww.score
	if score == nil {
		return
	}
	beatToX := func(beatf float64) int {
		frame, ok := score.ToFrame(beatf)
		if !ok {
			return r.Max.X * 2
		}
		return ww.PixelAtFrame(frame)
	}
	label := func(beatf float64) string {
		return fmt.Sprintf("b%d", int(beatf) + 1)
	}
	b0, _ := score.ToBeat(score.NearestBeat(ww.FrameAtPixel(r.Min.X)).frame)
	bN, _ := score.ToBeat(score.NearestBeat(ww.FrameAtPixel(r.Max.X)).frame)
	ww.drawTicks(dst, r, true, b0, bN, 1.0, beatToX, label)
}


/* assumes 'r' covers the widget's width */
func (ww *WaveWidget) drawTimeAxis(dst draw.Image, r image.Rectangle) {
	wav := ww.wav
	if wav == nil {
		return
	}
	tToX := func(t float64) int {
		return ww.PixelAtFrame(wav.FrameAtTime(time.Duration(t) * time.Second))
	}
	label := func(t float64) string {
		dur := time.Duration(t) * time.Second
		return fmt.Sprintf("%02d:%02d", int(dur.Minutes()), int(dur.Seconds()) % 60)
	}
	t0 := math.Trunc(ww.TimeAtCursor(0).Seconds())
	tN := math.Ceil(ww.TimeAtCursor(r.Dx()).Seconds())
	ww.drawTicks(dst, r, false, t0, tN, 1.0, tToX, label)
}
