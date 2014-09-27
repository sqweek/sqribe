package main

import (
	"image"
	"math/big"
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
	staff *Staff
}

type mouseState struct {
	cursor Cursor
	dragFn DragFn
	note *noteProspect
}

type FrameRange struct {
	min, max FrameN
}

type WaveWidget struct {
	/* data related state */
	wav *Waveform
	score *Score
	iolisten <-chan *Chunk

	/* view related state */
	first_frame FrameN
	frames_per_pixel int
	selection *FrameRange
	rect struct {
		r image.Rectangle		// the whole widget's rect
		wave image.Rectangle	// rect of the waveform display
		staves map[*Staff] image.Rectangle
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
	ww.rect.staves = make(map[*Staff]image.Rectangle)
	ww.selection = &FrameRange{0, 0}
	ww.renderstate.img = nil
	ww.renderstate.changed = WAV
	ww.refresh = refresh
	return &ww
}

func (ww *WaveWidget) Rect() image.Rectangle {
	return ww.rect.r
}

func (ww *WaveWidget) SelectAudio(start, end FrameN) {
	sel := FrameRange{start, end}
	ww.selection = &sel

	G.plumb.selection.C <- sel
	// XXX could avoid redrawing waveform if selection rendered differently
	ww.renderstate.changed |= WAV
	ww.refresh <- ww.rect.r
}

func (ww *WaveWidget) SelectAudioSnapToBeats(start, end FrameN) {
	score := ww.score
	if score == nil {
		ww.SelectAudio(start, end)
	} else {
		ww.SelectAudio(score.NearestBeat(start).frame, score.NearestBeat(end).frame)
	}
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
	dx := x - ww.rect.wave.Min.X
	return ww.first_frame + FrameN(dx * ww.frames_per_pixel)
}

func (ww *WaveWidget) PixelAtFrame(frame FrameN) int {
	/* TODO rounding */
	return ww.rect.wave.Min.X + int(frame - ww.first_frame) / ww.frames_per_pixel
}

func (ww *WaveWidget) dragBeat(min, max FrameN, beat *BeatRef) DragFn {
	var updateSelection func(FrameN) = nil
	if ww.selection.min != ww.selection.max {
		if beat.frame == ww.selection.min {
			updateSelection = func(f FrameN) {
				if f < ww.selection.max {
					ww.SelectAudio(f, ww.selection.max)
				}
			}
		} else if beat.frame == ww.selection.max {
			updateSelection = func(f FrameN) {
				if f > ww.selection.min {
					ww.SelectAudio(ww.selection.min, f)
				}
			}
		}
	}
	return func(pos image.Point, finished bool) bool {
		f := ww.FrameAtPixel(pos.X)
		if f <= min || f >= max {
			return false
		}
		orig := beat.frame
		if f != orig {
			beat.frame = f
			if updateSelection != nil {
				updateSelection(f)
			}
			ww.renderstate.changed |= SCALE
			ww.refresh <- ww.rect.r
		}
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
	for staff, rect := range ww.rect.staves {
		if !mouse.In(rect) {
			continue
		}
		mid := rect.Min.Y + rect.Dy() / 2
		for _, note := range(staff.notes) {
			frame, _ := ww.score.ToFrame(ww.score.Beatf(note))
			if frame < f0 || frame > fN {
				continue
			}
			x := ww.PixelAtFrame(frame)
			delta, _ := staff.LineForPitch(note.Pitch)
			y := mid - (yspacing / 2) * (delta)
			r := padPt(image.Pt(x, y), yspacing / 2, yspacing / 2)
			if mouse.In(r) {
				return func(pos image.Point, finished bool)bool {
					prospect := ww.noteAtPixel(staff, pos)
					if prospect == nil {
						return false
					}
					if finished {
						staff.RemoveNote(note)
						staff.AddNote(ww.mkNote(prospect))
					} else {
						ww.getMouseState(pos).note = prospect
					}
					ww.renderstate.changed |= SCALE
					ww.refresh <- ww.rect.r
					return true
				}, GrabCursor
			}
		}
	}

	// TODO ignore beat grabs when sufficiently zoomed out
	for i, beat := range(ww.score.beats) {
		min, max := FrameN(0), nframes
		if beat.frame < f0 {
			min = 0
			continue
		} else if beat.frame > fN {
			break
		}
		x := ww.PixelAtFrame(beat.frame)
		y0 := ww.rect.wave.Min.Y
		r := image.Rect(x, y0, x + 1, y0 + beatIncursion)
		if mouse.In(padRect(r, 2, 0)) {
			if i + 1 < len(ww.score.beats) {
				max = ww.score.beats[i + 1].frame
			}
			return ww.dragBeat(min, max, beat), ResizeHCursor
		}
	}

	snap := (mouse.Y - ww.rect.r.Min.Y < 4 * ww.rect.r.Dy() / 5)
	if mouse.In(padRect(ww.vrect(ww.PixelAtFrame(ww.selection.min)), 2, 0)) {
		return ww.selectDrag(ww.selection.max, snap), ResizeLCursor
	}
	if mouse.In(padRect(ww.vrect(ww.PixelAtFrame(ww.selection.max)), 2, 0)) {
		return ww.selectDrag(ww.selection.min, snap), ResizeRCursor
	}

	/* if we're not dragging anything in particular, drag to select */
	if mouse.In(ww.rect.wave) {
		return ww.selectDrag(ww.FrameAtPixel(mouse.X), snap), NormalCursor
	}
	return nil, NormalCursor
}

func (ww *WaveWidget) staffContaining(pos image.Point) *Staff {
	for staff, rect := range ww.rect.staves {
		if pos.In(rect) {
			return staff
		}
	}
	return nil
}

func (ww *WaveWidget) noteAtPixel(staff *Staff, pos image.Point) *noteProspect {
	rect := ww.rect.staves[staff]
	mid := rect.Min.Y + rect.Dy() / 2
	noteY := snapto(pos.Y, mid, yspacing / 2)
	delta := (mid - noteY) / (yspacing / 2)

	frame := ww.FrameAtPixel(pos.X)
	beatf, ok := ww.score.ToBeat(frame)
	if !ok {
		return nil
	}

	return &noteProspect{delta, beatf, staff}
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

	staff := ww.staffContaining(pos)
	if staff == nil {
		staff = ww.score.staves[0]
	}
	state.note = ww.noteAtPixel(staff, pos)

	return state
}

func (ww *WaveWidget) CursorIconAtPixel(mouse image.Point) (DragFn, Cursor) {
	s := ww.getMouseState(mouse)
	return s.dragFn, s.cursor
}

func (ww *WaveWidget) MouseMoved(mousePos image.Point) {
	if !mousePos.Eq(ww.mouse.pos) {
		ww.mouse.pos = mousePos
		ww.mouse.state = nil
		// XXX this could be less severe than CURSOR
		ww.renderstate.changed |= CURSOR
		ww.refresh <- ww.rect.r
	}
	if ww.cursorX != mousePos.X && mousePos.X > ww.rect.wave.Min.X {
		ww.cursorX = mousePos.X
		ww.renderstate.changed |= CURSOR
		ww.refresh <- ww.rect.r
	}
}

func (ww *WaveWidget) mkNote(prospect *noteProspect) *Note {
	beat, offset := ww.score.Quantize(prospect.beatf)
	return &Note{prospect.staff.PitchForLine(prospect.delta), ww.score.beatLen, beat, offset}
}

func (ww *WaveWidget) LeftClick(mouse image.Point) {
	indent := ww.rect.wave.Min.X - ww.rect.r.Min.X
	for staff, rect := range ww.rect.staves {
		if mouse.In(leftRect(rect, indent)) {
			staff.Muted = !staff.Muted
			ww.renderstate.changed |= SCALE
			ww.refresh <- ww.rect.r
		}
	}
}

func (ww *WaveWidget) RightClick(mouse image.Point) {
	s := ww.getMouseState(mouse)
	if s.note == nil || ww.score == nil {
		return
	}
	s.note.staff.AddNote(ww.mkNote(s.note))
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
	/* XXX should probably only account for cursor when mouse is over widget */
	x := ww.mouse.pos.X
	frameAtMouse := ww.FrameAtPixel(x)
	fpp := int(float64(ww.frames_per_pixel) * factor)
	if fpp < 1 {
		fpp = 1
	}
	delta := float64(fpp) / float64(ww.frames_per_pixel)
	if delta != 1.0 {
		dx := x - ww.rect.r.Min.X
		ww.first_frame = frameAtMouse - FrameN(dx * fpp)
		ww.frames_per_pixel = fpp
		ww.renderstate.changed |= WAV | CURSOR
		ww.mouse.state = nil
		ww.refresh <- ww.rect.r
	}
	return delta
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
	nsharps := 0
	if s.note != nil {
		beatf := s.note.beatf
		delta = s.note.delta
		beat, o := ww.score.Quantize(beatf)
		beati = beat.index
		offset = o
		pitch = s.note.staff.PitchForLine(delta)
		delta2, _ = s.note.staff.LineForPitch(pitch)
		nsharps = s.note.staff.nsharps
	}

	return fmt.Sprintf("line=%d (%d) pitch=%d %d pos=%d:%v #%d", delta, delta2, pitch, pitch%12, beati, offset, nsharps)
}
