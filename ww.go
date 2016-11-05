package main

import (
	"image"
	"math/big"
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

type FramePos struct {
	f0 FrameN
	ppix int
}

type WaveWidget struct {
	WidgetCore

	/* data related state */
	wav *wave.Waveform
	score *score.Score
	iolisten <-chan *wave.Chunk

	/* view related state */
	pos FramePos
	selection TimeRange
	rect WaveLayout
	notesel map[*score.Note]*score.Staff
	snarf map[*score.Staff] []*score.Note // the cut/copy buffer
	pasteMode bool
	beatdrag map[*score.BeatRef]FrameN

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
	ww.pos.f0 = 0
	ww.pos.ppix = 512
	ww.rect.staff.Store(make(map[*score.Staff]*StaffLayout))
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
					frng:= ww.pos.Range(ww.rect.wave.Dx())
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
					gone := make([]score.StaffNote, 0, 8)
					for note, staff := range ww.notesel {
						if _, ok := ev.Staves[staff]; !ok {
							continue
						}
						if staff.NoteAt(note) != note {
							/* note has been removed from staff */
							gone = append(gone, score.StaffNote{staff, note})
						}
					}
					ww.deselectNotes(gone...)
					if len(sc.Staves()) != len(ww.rect.staves()) {
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

func (pos *FramePos) Range(dx int) FrameRange {
	return FrameRange{pos.f0, pos.f0 + FrameN(pos.ppix * dx)}
}

func (ww *WaveWidget) SetCursorByFrame(frame FrameN, follow bool) {
	xmin, xmax := ww.rect.wave.Min.X, ww.rect.wave.Max.X
	x := xmin + ww.pos.DxAtFrame(frame)
	if follow && (x < xmin || x > xmax) {
		ww.ScrollToPixel(x)
		x = xmin + ww.pos.DxAtFrame(frame)
	}
	ww.cursorX = x
	ww.changed(CURSOR, frame)
}

func (ww *WaveWidget) WaveRange() TimeRange {
	return wave.Range(ww.wav)
}

func (ww *WaveWidget) FrameAtCursor() FrameN {
       return ww.pos.FrameAtDx(ww.cursorX - ww.rect.wave.Min.X)
}

func (ww *WaveWidget) FrameAtPixel(x int) FrameN {
	return ww.pos.FrameAtDx(x - ww.rect.wave.Min.X)
}

func (pos *FramePos) FrameAtDx(dx int) FrameN {
	return pos.f0 + FrameN(dx * pos.ppix + pos.ppix/2)
}

func (pos *FramePos) DxAtFrame(frame FrameN) int {
	return int(frame - pos.f0) / pos.ppix
}

func (ww *WaveWidget) areAllSelected(notes... score.StaffNote) bool {
	for _, sn := range notes {
		if _, ok := ww.notesel[sn.Note]; !ok {
			return false
		}
	}
	return true
}

func (ww *WaveWidget) toggleNotes(clear bool, notes... score.StaffNote) {
	allSelected := !clear && ww.areAllSelected(notes...)
	if allSelected {
		ww.deselectNotes(notes...)
	} else {
		ww.selectNotes(clear, notes...)
	}
}

func (ww *WaveWidget) deselectNotes(notes... score.StaffNote) {
	newsel := make(map[*score.Note]*score.Staff)
	for note, staff := range ww.notesel {
		newsel[note] = staff
	}
	for _, sn := range notes {
		delete(newsel, sn.Note)
	}
	ww.notesel = newsel
}

func (ww *WaveWidget) selectNotes(clear bool, notes... score.StaffNote) {
	newsel := make(map[*score.Note]*score.Staff)
	if !clear {
		for note, staff := range ww.notesel {
			newsel[note] = staff
		}
	}
	for _, sn := range notes {
		newsel[sn.Note] = sn.Staff
	}
	ww.notesel = newsel
}

func (ww *WaveWidget) staffContaining(pos image.Point) (*score.Staff, *StaffLayout) {
	for staff, slayout := range ww.rect.staves() {
		if !slayout.mix.Minimised && pos.In(slayout.r) {
			return staff, slayout
		}
	}
	return nil, nil
}

func (ww *WaveWidget) noteAtPixel(staff *score.Staff, pos image.Point) *noteProspect {
	if slayout, ok := ww.rect.staves()[staff]; ok {
		return ww.noteAtPixelWithMid(staff, pos, slayout.Mid())
	}
	return nil
}

func (ww *WaveWidget) noteAtPixelWithMid(staff *score.Staff, pos image.Point, mid int) *noteProspect {
	noteY := snapto(pos.Y, mid, yspacing / 2)
	delta := (mid - noteY) / (yspacing / 2)

	frame := ww.FrameAtPixel(pos.X)

	if beatf, ok := ww.score.ToBeat(frame); ok {
		return &noteProspect{delta, beatf, staff}
	}
	return nil
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

	if staff, slayout := ww.staffContaining(pos); staff != nil {
		state.note = ww.noteAtPixelWithMid(staff, pos, slayout.Mid())
	} else {
		state.note = nil;
	}
	return state
}

func (ww *WaveWidget) ScrollToFrame(f FrameN) int {
	return ww.ScrollToPixel(ww.rect.wave.Min.X + ww.pos.DxAtFrame(f))
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
	shift := FrameN(dx * ww.pos.ppix)
	target := ww.wav.Clip(ww.pos.f0 + shift, FrameN((ww.rect.wave.Dx() + 1) * ww.pos.ppix))
	if target == ww.pos.f0 {
		return 0
	}
	ww.pos.f0 = target
	ww.mouse.state = nil
	ww.changed(WAV | CURSOR | VIEWPOS, &ww.pos)
	return int(target - ww.pos.f0)
}

func (ww *WaveWidget) Zoom(factor float64) float64 {
	fpp := int(float64(ww.pos.ppix) * factor)
	if fpp == ww.pos.ppix {
		if factor < 1.0 {
			fpp--
		} else if factor > 1.0 {
			fpp++
		}
	}
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
	delta := float64(fpp) / float64(ww.pos.ppix)
	if delta != 1.0 {
		/* XXX should probably only account for cursor when mouse is over widget */
		x := ww.mouse.pos.X
		frameAtMouse := ww.FrameAtPixel(x)
		dx := x - ww.rect.wave.Min.X
		ww.pos.f0 = frameAtMouse - FrameN(dx * fpp)
		ww.pos.ppix = fpp
		ww.mouse.state = nil
		ww.changed(WAV | CURSOR | VIEWPOS, &ww.pos)
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

func (ww *WaveWidget) beatFrame(beat *score.BeatRef) FrameN {
	if ww.beatdrag != nil {
		if f, ok := ww.beatdrag[beat]; ok {
			return f
		}
	}
	return beat.Frame()
}

func (ww *WaveWidget) ToFrame(pt score.BeatPoint) FrameN {
	b1 := pt.Beat()
	f1, f2 := ww.beatFrame(b1), ww.beatFrame(b1.LNext())
	return f1 + FrameN(pt.Offsetf() * float64(f2 - f1))
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
