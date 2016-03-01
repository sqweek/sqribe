package main

import (
	"image"
	"math/big"
	"time"
	"fmt"

	"github.com/skelterjohn/go.wde"

	"github.com/sqweek/sqribe/midi"
	"github.com/sqweek/sqribe/score"
	"github.com/sqweek/sqribe/wave"

	. "github.com/sqweek/sqribe/core/types"
)

type changeMask int

const (
	WAV changeMask = 1 << iota
	SELXN
	MIXER
	SCALE
	CURSOR
	BEATS
	VIEWPOS
	LAYOUT
	MAXBIT
	RESET // clears layout state - not included in EVERYTHING
	EVERYTHING changeMask = MAXBIT - 1
)

const yspacing = 12 // pixels between staff lines

type noteProspect struct {
	delta int
	beatf score.BeatPoint
	staff *score.Staff
}

func (n *noteProspect) Eq(n2 *noteProspect) bool {
	return n.staff == n2.staff && n.delta == n2.delta && n.beatf == n2.beatf
}

func (p *noteProspect) Δpitch(note *score.Note) int8 {
	nline, _ := p.staff.LineForPitch(note.Pitch)
	if nline == p.delta {
		return 0
	}
	return int8(p.staff.PitchForLine(p.delta) - note.Pitch)
}

/* mkNote returns an existing note on the same staff line, if it exists (duration is ignored).
 * Otherwise a new note is created with the given duration. */
func (p *noteProspect) mkNote(sc *score.Score, duration *big.Rat) (*score.Note, bool) {
	beat, offset := sc.Quantize(p.beatf)
	f := beat.FrameAtRat(offset)
	next := sc.Iter(FrameRange{f, f}, p.staff)
	var sn score.StaffNote
	for next != nil {
		sn, next = next()
		if p.Δpitch(sn.Note) == 0 {
			return sn.Note, true
		}
	}
	/* no existing note found */
	return &score.Note{p.staff.PitchForLine(p.delta), duration, beat, offset}, false
}

type noteDrag struct {
	Δpitch int8
	Δbeat *big.Rat
}

type mouseState struct {
	cursor wde.Cursor
	dragFn DragFn
	note *noteProspect
	ndelta *noteDrag
	rectSelect *image.Rectangle
}

type WaveWidget struct {
	WidgetCore

	/* data related state */
	wav *wave.Waveform
	score *score.Score
	iolisten <-chan *wave.Chunk

	/* view related state */
	first_frame FrameN
	frames_per_pixel int
	selection TimeRange
	rect WaveLayout
	notesel map[*score.Note]*score.Staff
	snarf map[*score.Staff] []*score.Note // the cut/copy buffer
	pasteMode bool

	/* renderer related state */
	renderstate struct {
		img *image.RGBA
		waveRulers *image.RGBA
		changed changeMask
		cursor *image.RGBA
		cursorPrevX int
	}
	mouse struct {
		pos image.Point
		state *mouseState
	}
	cursorX int
}

func NewWaveWidget(refresh chan Widget) *WaveWidget {
	var ww WaveWidget
	ww.first_frame = 0
	ww.frames_per_pixel = 512
	ww.rect.staves = make(map[*score.Staff]image.Rectangle)
	ww.rect.mixers = make(map[*score.Staff]*MixerLayout)
	ww.selection = &FrameRange{0, 0}
	ww.notesel = make(map[*score.Note]*score.Staff)
	ww.renderstate.img = nil
	ww.renderstate.changed = WAV
	ww.refresh = refresh
	return &ww
}

func (ww *WaveWidget) changed(mask changeMask, ev interface{}) {
	ww.renderstate.changed |= mask
	ww.refresh <- ww
}

func (ww *WaveWidget) SelectAudio(sel TimeRange) {
	ww.selection = sel
	G.plumb.selection.C <- sel
	ww.changed(SELXN, sel)
}

func (ww *WaveWidget) SelectAudioSnapToBeats(start, end FrameN) {
	sc := ww.score
	if sc == nil {
		ww.SelectAudio(FrameRange{start, end})
	} else {
		beats := score.BeatRange{sc.NearestBeat(start), sc.NearestBeat(end)}
		ww.SelectAudio(beats)
	}
}

func (ww *WaveWidget) ShuntSel(Δbeat int) {
	sc := ww.score
	br, ok := ww.selection.(score.BeatRange)
	if ok && sc != nil {
		ww.SelectAudio(sc.Shunt(br, Δbeat))
	}
}

func (ww *WaveWidget) SelectedTimeRange() TimeRange {
	return ww.selection
}

func (ww *WaveWidget) SetWaveform(wav *wave.Waveform) *wave.Waveform {
	old := ww.wav
	if old != nil {
		old.CacheIgnore(ww.iolisten)
	}
	ww.wav = wav
	if wav != nil {
		iolisten := wav.CacheListen()
		ww.iolisten = iolisten
		go func() {
			for {
				chunk, ok := <-iolisten
				if !ok {
					return
				}
				if chunk == nil {
					ww.ScrollPixels(0)
				} else {
					frng:= ww.VisibleFrameRange()
					s0, sN := wav.SampleRange(frng.MinFrame(), frng.MaxFrame())
					if chunk.Intersects(s0, sN) {
						ww.changed(WAV, chunk)
					}
				}
			}
		}()
	}
	ww.changed(WAV | VIEWPOS, wav)
	return old
}

func (ww *WaveWidget) SetScore(sc *score.Score) *score.Score {
	old := ww.score
	if old != nil {
		old.Unsub(ww)
	}
	ww.score = sc
	if sc != nil {
		events := make(chan interface{})
		ww.score.Sub(ww, events)
		go func() {
			for ev := range events {
				change := SCALE
				switch ev := ev.(type) {
				case score.BeatChanged:
					change |= BEATS
				case score.KeyChanged:
					change |= MIXER
				case score.StaffChanged:
					for note, staff := range ww.notesel {
						if _, ok := ev.Staves[staff]; !ok {
							continue
						}
						if staff.NoteAt(note) != note {
							/* note has been removed from staff */
							delete(ww.notesel, note)
						}
					}
					if len(sc.Staves()) != len(ww.rect.staves) {
						change |= LAYOUT
					}
				case score.ResetStaves:
					ww.selectNotes(true) // clear selection
					change |= RESET
				}
				// XXX could avoid redraw if the staff/beats aren't visible...
				ww.changed(change, ev)
			}
		}()
		selxn := make(chan interface{})
		G.plumb.selection.Sub(&sc, selxn)
		sc.InitQuantizer(selxn)
	}
	ww.changed(SCALE | LAYOUT, sc)
	return old
}

func (ww *WaveWidget) SelectedNotes() []score.StaffNote {
	notes := make([]score.StaffNote, 0, len(ww.notesel))
	for note, staff := range ww.notesel {
		notes = append(notes, score.StaffNote{staff, note})
	}
	return notes
}

func (ww *WaveWidget) VisibleFrameRange() FrameRange {
	w0 := ww.first_frame
	wN := w0 + FrameN(ww.frames_per_pixel) * FrameN(ww.rect.wave.Dx())
	return FrameRange{w0, wN}
}

func (ww *WaveWidget) SetCursorByFrame(frame FrameN, follow bool) {
	x := ww.PixelAtFrame(frame)
	if follow && (x < ww.rect.wave.Min.X || x > ww.rect.wave.Max.X) {
		ww.ScrollToPixel(x)
		x = ww.PixelAtFrame(frame)
	}
	ww.cursorX = x
	ww.changed(CURSOR, frame)
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

func (ww *WaveWidget) allAreSelected(notes... score.StaffNote) bool {
	for _, sn := range notes {
		if _, ok := ww.notesel[sn.Note]; !ok {
			return false
		}
	}
	return true
}

func (ww *WaveWidget) selectNotes(clear bool, notes... score.StaffNote) {
	if clear {
		for note, _ := range ww.notesel {
			delete(ww.notesel, note)
		}
	}
	allSelected := ww.allAreSelected(notes...)
	for _, sn := range notes {
		if allSelected {
			delete(ww.notesel, sn.Note)
		} else {
			ww.notesel[sn.Note] = sn.Staff
		}
	}
}

func (ww *WaveWidget) staffContaining(pos image.Point) *score.Staff {
	for staff, rect := range ww.rect.staves {
		if pos.In(rect) {
			return staff
		}
	}
	return nil
}

func (ww *WaveWidget) noteAtPixel(staff *score.Staff, pos image.Point) *noteProspect {
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
		state.note = nil;
	} else {
		state.note = ww.noteAtPixel(staff, pos)
	}

	return state
}

func (ww *WaveWidget) ScrollToFrame(f FrameN) int {
	return ww.ScrollToPixel(ww.PixelAtFrame(f))
}

func (ww *WaveWidget) ScrollToPixel(x int) int {
	return ww.ScrollPixels(x - ww.rect.wave.Min.X - int(0.05 * float64(ww.rect.wave.Dx())))
}

func (ww *WaveWidget) Scroll(amount float64) int {
	return ww.ScrollPixels(int(float64(ww.rect.wave.Dx()) * amount))
}

func (ww *WaveWidget) ScrollPixels(dx int) int {
	if ww.Rect().Empty() || ww.wav == nil {
		return 0
	}
	shift := FrameN(dx * ww.frames_per_pixel)
	target := ww.wav.Clip(ww.first_frame + shift, FrameN((ww.rect.wave.Dx() + 1) * ww.frames_per_pixel))
	if target == ww.first_frame {
		return 0
	}
	ww.first_frame = target
	ww.mouse.state = nil
	ww.changed(WAV | CURSOR | VIEWPOS, ww.first_frame)
	return int(target - ww.first_frame)
}

func (ww *WaveWidget) Zoom(factor float64) float64 {
	fpp := int(float64(ww.frames_per_pixel) * factor)
	if fpp < 1 {
		fpp = 1
	}
	if wav := ww.wav; wav != nil && ww.rect.wave.Dx() > 0 {
		max_frames := (12*1024*1024 / 10 / 2 / 2) * 9
		max_fpp := int(max_frames / ww.rect.wave.Dx())
		if fpp > max_fpp {
			fpp = max_fpp
		}
	}
	delta := float64(fpp) / float64(ww.frames_per_pixel)
	if delta != 1.0 {
		/* XXX should probably only account for cursor when mouse is over widget */
		x := ww.mouse.pos.X
		frameAtMouse := ww.FrameAtPixel(x)
		dx := x - ww.rect.wave.Min.X
		ww.first_frame = frameAtMouse - FrameN(dx * fpp)
		ww.frames_per_pixel = fpp
		ww.mouse.state = nil
		ww.changed(WAV | CURSOR | VIEWPOS, fpp)
	}
	return delta
}

func (ww *WaveWidget) Snarf() {
	snarf := make(map[*score.Staff] []*score.Note)
	for note, staff := range ww.notesel {
		snarf[staff] = score.Merge(snarf[staff], note.Dup())
	}
	ww.snarf = snarf
}

func (ww *WaveWidget) PasteMode() bool {
	return ww.pasteMode
}

func (ww *WaveWidget) SetPasteMode(mode bool) {
	if ww.pasteMode == mode || (mode && ww.snarf == nil) {
		return
	}
	ww.pasteMode = mode
	ww.changed(SCALE, ww.snarf)
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
	offset := big.NewRat(1, 1)
	nsharps := score.KeySig(-99)
	if s.note != nil {
		beatf := s.note.beatf
		delta = s.note.delta
		_, offset = ww.score.Quantize(beatf)
		pitch = s.note.staff.PitchForLine(delta)
		delta2, _ = s.note.staff.LineForPitch(pitch)
		nsharps = ww.score.Key()
	}

	return fmt.Sprintf("line=%d (%d) pitch=%d %s offset=%v %v %v", delta, delta2, pitch, midi.PitchName(pitch), offset, nsharps, len(ww.notesel))
}
