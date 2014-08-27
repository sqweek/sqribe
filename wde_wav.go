package main

import (
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

const beatIncursion = 5 // pixels

const yspacing = 10 // pixels between staff lines

type noteProspect struct {
	delta int
	beatf float64
}

type mouseState struct {
	cursor Cursor
	dragFn DragFn
	note *noteProspect
}

type WaveWidget struct {
	/* data related state */
	wav *Waveform
	score *Score
	iolisten <-chan *Chunk

	/* view related state */
	first_frame FrameN
	frames_per_pixel int
	selection struct {
		min FrameN
		max FrameN
	}
	rect struct {
		r image.Rectangle		// the whole widget's rect
		wave image.Rectangle	// rect of the waveform display
	}

	/* renderer related state */
	renderstate struct {
		img *image.RGBA
		waveform *image.RGBA
		changed changeMask
	}
	mouse struct {
		pos image.Point
		state *mouseState
	}
	cursorX int
	refresh chan image.Rectangle
}

func NewWaveWidget(refresh chan image.Rectangle) *WaveWidget {
	var ww WaveWidget
	ww.first_frame = 0
	ww.frames_per_pixel = 512
	ww.rect.r = image.Rect(0,0,0,0)
	ww.renderstate.img = nil
	ww.renderstate.changed = WAV
	ww.refresh = refresh
	return &ww
}

func (ww *WaveWidget) Rect() image.Rectangle {
	return ww.rect.r
}

func (ww *WaveWidget) SelectAudio(start, end FrameN) {
	if ww.wav == nil {
		return
	}
	ww.selection.min = start
	ww.selection.max = end
	ww.renderstate.changed |= WAV // XXX doesn't really need to redraw waveform
	ww.refresh <- ww.rect.r
}

func (ww *WaveWidget) SelectAudioSnapToBeats(start, end FrameN) {
	if ww.wav == nil || ww.score == nil {
		return
	}
	ww.selection.min = ww.score.NearestBeat(start)
	ww.selection.max = ww.score.NearestBeat(end)
	// XXX could avoid redrawing waveform if selection rendered differently
	ww.renderstate.changed |= WAV
	ww.refresh <- ww.rect.r
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
					ww.refresh <- ww.rect.r
				}
			}
		}()
	}
	ww.renderstate.changed |= WAV
	ww.refresh <- ww.rect.r
}

func (ww *WaveWidget) SetScore(score *Score) {
	ww.score = score
	//TODO listener stuff
}

func (ww *WaveWidget) VisibleFrameRange() (FrameN, FrameN) {
	w0 := ww.first_frame
	wN := w0 + FrameN(ww.frames_per_pixel) * FrameN(ww.rect.wave.Dx())
	return w0, wN
}

func (ww *WaveWidget) SetCursorByFrame(frame FrameN) {
	ww.cursorX = ww.PixelAtFrame(frame)
	ww.renderstate.changed |= CURSOR
	ww.refresh <- ww.rect.r
}

func (ww *WaveWidget) NFrames() FrameN {
	if ww.wav == nil {
		/* TODO allow score without wave */
		return 0
	}
	return ww.wav.ToFrame(ww.wav.NSamples)
}

func (ww *WaveWidget) FrameAtCursor() FrameN {
       return ww.FrameAtPixel(ww.cursorX)
}

func (ww *WaveWidget) FrameAtPixel(x int) FrameN {
	dx := x - ww.rect.r.Min.X
	return ww.first_frame + FrameN(dx * ww.frames_per_pixel)
}

func (ww *WaveWidget) PixelAtFrame(frame FrameN) int {
	/* TODO rounding */
	return ww.rect.r.Min.X + int(frame - ww.first_frame) / ww.frames_per_pixel
}

func (ww *WaveWidget) dragFn(min, max FrameN, ptr *FrameN, cm changeMask) DragFn {
	return func(pos image.Point, finished bool) bool {
		f := ww.FrameAtPixel(pos.X)
		if f <= min || f >= max {
			return false
		}
		*ptr = f
		ww.renderstate.changed |= cm
		ww.refresh <- ww.rect.r
		return true
	}
}

func (ww *WaveWidget) selectDrag(anchor FrameN, snap bool) DragFn {
	return func(pos image.Point, finished bool)bool {
		min := ww.FrameAtPixel(pos.X)
		max := anchor
		if max < min {
			min, max = max, min
		}
		if snap {
			ww.SelectAudioSnapToBeats(min, max)
		} else {
			ww.SelectAudio(min, max)
		}
		return true
	}
}

func padPt(center image.Point, w, h int) image.Rectangle {
	return image.Rect(center.X - w, center.Y - h, center.X + w + 1, center.Y + h + 1)
}

func padRect(r image.Rectangle, w, h int) image.Rectangle {
	return image.Rect(r.Min.X - w, r.Min.Y - h, r.Max.X + w, r.Max.Y + h)
}

func (ww *WaveWidget) vrect(x int) image.Rectangle{
	return image.Rect(x, ww.rect.r.Min.Y, x + 1, ww.rect.r.Max.Y)
}

func (ww *WaveWidget) dragState(mouse image.Point) (DragFn, Cursor) {
	nframes := ww.NFrames()

	f0, fN := ww.VisibleFrameRange()
	for _, note := range(ww.score.notes) {
		frame, _ := ww.score.ToFrame(note.Beatf())
		if frame < f0 || frame > fN {
			continue
		}
		x := ww.PixelAtFrame(frame)
		mid := ww.rect.wave.Min.Y + ww.rect.wave.Dy() / 2
		delta, _ := ww.score.LineForPitch(note.Pitch)
		y := mid - (yspacing / 2) * (delta)
		r := padPt(image.Pt(x, y), yspacing / 2, yspacing / 2)
		if mouse.In(r) {
			return func(pos image.Point, finished bool)bool {
				prospect := ww.noteAtPixel(pos)
				if prospect == nil {
					return false
				}
				if finished {
					ww.score.RemoveNote(note)
					ww.score.AddNote(ww.mkNote(prospect))
				} else {
					ww.getMouseState(pos).note = prospect
				}
				ww.renderstate.changed |= SCALE
				ww.refresh <- ww.rect.r
				return true
			}, GrabCursor
		}
	}

	snap := (mouse.Y - ww.rect.r.Min.Y < 4 * ww.rect.r.Dy() / 5)
	if mouse.In(padRect(ww.vrect(ww.PixelAtFrame(ww.selection.min)), 2, 0)) {
		return ww.selectDrag(ww.selection.max, snap), ResizeLCursor
	}
	if mouse.In(padRect(ww.vrect(ww.PixelAtFrame(ww.selection.max)), 2, 0)) {
		return ww.selectDrag(ww.selection.min, snap), ResizeRCursor
	}

	// TODO ignore beat grabs when sufficiently zoomed out
	for i, beat := range(ww.score.beats) {
		min, max := FrameN(0), nframes
		if beat < f0 {
			min = 0
			continue
		} else if beat > fN {
			break
		}
		x := ww.PixelAtFrame(beat)
		y0 := ww.rect.r.Min.Y
		r := image.Rect(x, y0, x + 1, y0 + beatIncursion)
		if mouse.In(padRect(r, 2, 0)) {
			if i + 1 < len(ww.score.beats) {
				max = ww.score.beats[i + 1]
			}
			return ww.dragFn(min, max, &ww.score.beats[i], SCALE), ResizeHCursor
		}
	}

	/* if we're not dragging anything in particular, drag to select */
	return ww.selectDrag(ww.FrameAtPixel(mouse.X), snap), NormalCursor
}

func (ww *WaveWidget) noteAtPixel(pos image.Point) *noteProspect {
	mid := ww.rect.wave.Min.Y + ww.rect.wave.Dy() / 2
	noteY := snapto(pos.Y, mid, yspacing / 2)
	delta := (mid - noteY) / (yspacing / 2)

	frame := ww.FrameAtPixel(pos.X)
	beatf, ok := ww.score.ToBeat(frame)
	if !ok {
		return nil
	}

	return &noteProspect{delta, beatf}
}

func (ww *WaveWidget) getMouseState(pos image.Point) *mouseState {
	state := ww.mouse.state
	cachedPos := ww.mouse.pos
	if state != nil && pos.Eq(cachedPos) {
		return state
	}
	state = ww.calcMouseState(pos)
	ww.mouse.state = state
	ww.mouse.pos = pos

	return state
}

func (ww *WaveWidget) calcMouseState(pos image.Point) *mouseState {
	state := new(mouseState)

	dragFn, cursor := ww.dragState(pos)
	state.dragFn = dragFn
	state.cursor = cursor

	state.note = ww.noteAtPixel(pos)

	return state
}

func (ww *WaveWidget) CursorIconAtPixel(mouse image.Point) (DragFn, Cursor) {
	s := ww.getMouseState(mouse)
	return s.dragFn, s.cursor
}

func (ww *WaveWidget) SetCursorByPixel(mousePos image.Point) {
	if !mousePos.Eq(ww.mouse.pos) {
		ww.mouse.pos = mousePos
		ww.mouse.state = nil
		// XXX this could be less severe than CURSOR
		ww.renderstate.changed |= CURSOR
		ww.refresh <- ww.rect.r
	}
	if ww.cursorX != mousePos.X {
		ww.cursorX = mousePos.X
		ww.renderstate.changed |= CURSOR
		ww.refresh <- ww.rect.r
	}
}

func (ww *WaveWidget) mkNote(prospect *noteProspect) *Note {
	beati, offset := ww.score.Quantize(prospect.beatf)
	b := big.NewRat(int64(beati), 1)
	offset.Mul(big.NewRat(ww.score.beatLen.Denom().Int64(), 1), offset)
	b.Add(b, offset)
	return &Note{ww.score.PitchForLine(prospect.delta), ww.score.beatLen, b}
}

func (ww *WaveWidget) LeftClick(mouse image.Point) {
}

func (ww *WaveWidget) RightClick(mouse image.Point) {
	s := ww.getMouseState(mouse)
	if s.note == nil || ww.score == nil {
		return
	}
	ww.score.AddNote(ww.mkNote(s.note))
}

func (ww *WaveWidget) Scroll(amount float64) int {
	if ww.rect.r.Empty() || ww.wav == nil {
		return 0
	}
	original := ww.first_frame
	width := ww.rect.r.Dx()
	shift := FrameN((float64(width) * amount) * float64(ww.frames_per_pixel))
	rbound := ww.NFrames() - FrameN((width + 1) * ww.frames_per_pixel)
	ww.first_frame += shift
	if ww.first_frame < 0 || rbound < 0 {
		ww.first_frame = 0
	} else if ww.first_frame > rbound {
		ww.first_frame = rbound
	}
	diff := int(ww.first_frame - original)
	if diff != 0 {
		ww.renderstate.changed |= WAV | CURSOR
		ww.mouse.state = nil
		ww.refresh <- ww.rect.r
	}
	return diff
}

func (ww *WaveWidget) Zoom(factor float64) float64 {
	/* TODO preserve mouse position */
	original := float64(ww.frames_per_pixel)
	ww.frames_per_pixel = int(original * factor)
	if ww.frames_per_pixel < 1 {
		ww.frames_per_pixel = 1
	}
	delta := float64(ww.frames_per_pixel) / original
	if delta != 1.0 {
		ww.renderstate.changed |= WAV | CURSOR
		ww.mouse.state = nil
		ww.refresh <- ww.rect.r
	}
	return delta
}

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

	// this is a bit too close to the blue right next door
	case "1/24": fallthrough
	case "3/24": fallthrough
	case "5/24": return color.RGBA{0x88, 0x00, 0x88, α}

	case "1/12": fallthrough
	case "1/6": return color.RGBA{0xff, 0x00, 0xff, α}
	}
	return color.RGBA{0x00, 0x00, 0x00, α}
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
		}
	}
}

func (ww *WaveWidget) TimeAtCursor(x int) time.Duration {
	if ww.wav == nil {
		return 0.0
	}
	frame := ww.FrameAtPixel(x)
	return ww.wav.TimeAtFrame(frame)
}

func (ww *WaveWidget) Status() string {
	s := ww.getMouseState(ww.mouse.pos)
	pitch := uint8(0)
	delta := 0
	delta2 := 0
	beati := 0
	offset := big.NewRat(1, 1)
	if s.note != nil {
		beatf := s.note.beatf
		delta = s.note.delta
		beati, offset = ww.score.Quantize(beatf)
		pitch = ww.score.PitchForLine(delta)
		delta2, _ = ww.score.LineForPitch(pitch)
	}

	return fmt.Sprintf("line=%d (%d) pitch=%d %d pos=%d:%v #%d", delta, delta2, pitch, pitch%12, beati, offset, ww.score.nsharps)
}
