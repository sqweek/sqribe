package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
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
		ww.rect.r = r
		ww.rect.wave = image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y - 20)
		ww.renderstate.img = image.NewRGBA(ww.rect.r)
		if ww.wav != nil {
			if change & WAV != 0 || ww.renderstate.waveform == nil {
				ww.renderstate.waveform = image.NewRGBA(ww.rect.wave)
				ww.drawWave(ww.renderstate.waveform, ww.rect.wave)
			}
			draw.Draw(ww.renderstate.img, ww.rect.wave, ww.renderstate.waveform, r.Min, draw.Src)
		}
		ww.drawScale(ww.renderstate.img, ww.rect.wave)

		curcol := color.RGBA{0, 0xdd, 0, 255}
		draw.Draw(ww.renderstate.img, image.Rect(ww.cursorX, r.Min.Y, ww.cursorX+1, r.Max.Y), &image.Uniform{curcol}, r.Min, draw.Src)
		axish := 20
		//ww.drawBeatAxis(ww.renderstate.img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y + axish))
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

func (ww *WaveWidget) drawScale(dst draw.Image, r image.Rectangle) {
	if ww.score == nil {
		return
	}
	black4 := color.RGBA{0x00, 0x00, 0x00, 0x88}
	black1 := color.RGBA{0x00, 0x00, 0x00, 0x22}
	_, lastFrame := ww.VisibleFrameRange()
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
		x := ww.PixelAtFrame(beat)
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
		ww.drawNote(dst, r, mid, note.Beatf(), delta, accidental, false)
	}
}

func (ww *WaveWidget) drawProspectiveNote(dst draw.Image, r image.Rectangle, mid int) {
	s := ww.getMouseState(ww.mouse.pos)
	if s.note == nil {
		return
	}
	n := ww.mkNote(s.note)
	ww.drawNote(dst, r, mid, n.Beatf(), s.note.delta, nil, true)
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

/* assumes 'r' covers the widget's width */
func (ww *WaveWidget) drawTimeAxis(dst draw.Image, r image.Rectangle) {
	targetPixPerTick := 30.0
	bg := color.RGBA{0x55, 0x44, 0x44, 0xff}
	fg := color.RGBA{0xcc, 0xcc, 0xbb, 0xff}
	minT := ww.TimeAtCursor(0).Seconds()
	maxT := ww.TimeAtCursor(r.Dx()).Seconds()
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Over)
	pixPerSecond := float64(r.Dx()) / (maxT - minT)
	dTmaj := 1.0
	minPerMaj := 0
	if (pixPerSecond < targetPixPerTick) {
		dTmaj = math.Trunc(0.5 + targetPixPerTick / pixPerSecond)
	} else {
		minPerMaj = int(pixPerSecond / targetPixPerTick) - 1
	}
	lastTextX := r.Min.X - 30
	for t := math.Trunc(minT); t <= maxT; t += dTmaj {
		for i := 1; i <= minPerMaj; i++ {
			tm := t + float64(i) * (dTmaj / float64(minPerMaj + 1))
			if tm >= minT && tm <= maxT {
				x := r.Min.X + int(0.5 + (tm - minT) * pixPerSecond)
				draw.Draw(dst, image.Rect(x, r.Min.Y, x+1, r.Min.Y + 4), &image.Uniform{fg}, image.ZP, draw.Over)
			}
		}
		if t >= minT && t <= maxT {
			x := r.Min.X + int(0.5 + (t - minT) * pixPerSecond)
			draw.Draw(dst, image.Rect(x, r.Min.Y, x+1, r.Min.Y + 7), &image.Uniform{fg}, image.ZP, draw.Over)
			if (x > lastTextX + 50) {
				lastTextX = x
				dur := time.Duration(t) * time.Second
				str := fmt.Sprintf("%02d:%02d", int(dur.Minutes()), int(dur.Seconds()) % 60)
				G.font.luxi.DrawC(dst, fg, r, str, image.Point{x, r.Min.Y + 14})
			}
		}
	}
}
