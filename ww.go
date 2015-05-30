package main

import (
	"image"
	"math/big"
	"time"
	"fmt"

	"github.com/sqweek/go.wde"

	"sqweek.net/sqribe/audio"
	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/score"
	"sqweek.net/sqribe/wave"

	. "sqweek.net/sqribe/core/types"
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
	EVERYTHING changeMask = MAXBIT - 1
)

const beatIncursion = 5 // pixels

const yspacing = 12 // pixels between staff lines

type noteProspect struct {
	delta int
	beatf float64
	staff *score.Staff
}

type noteDrag struct {
	Δpitch uint8
	Δbeat float64
}

type mouseState struct {
	cursor wde.Cursor
	dragFn DragFn
	note *noteProspect
	ndelta *noteDrag
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

	/* renderer related state */
	renderstate struct {
		img *image.RGBA
		waveRulers *image.RGBA
		changed changeMask
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

func (ww *WaveWidget) SelectAudio(sel TimeRange) {
	ww.selection = sel
	G.plumb.selection.C <- sel
	ww.renderstate.changed |= SELXN
	ww.refresh <- ww
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

func (ww *WaveWidget) GetSelectedTimeRange() TimeRange {
	return ww.selection
}

func (ww *WaveWidget) SetWaveform(wav *wave.Waveform) {
	if ww.wav != nil {
		ww.wav.CacheIgnore(ww.iolisten)
	}
	ww.wav = wav
	if ww.wav != nil {
		iolisten := ww.wav.CacheListen()
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
					ww.publish(chunk)
				}
			}
		}()
	}
	ww.renderstate.changed |= WAV | VIEWPOS
	ww.publish(wav)
}

func (ww *WaveWidget) SetScore(sc *score.Score) {
	if ww.score != nil {
		ww.score.Unsub(ww)
	}
	ww.score = sc
	if sc != nil {
		events := make(chan interface{})
		ww.score.Sub(ww, events)
		go func() {
			for ev := range events {
				if _, ok := ev.(score.BeatChanged); ok {
					ww.renderstate.changed |= BEATS
				}
				// XXX could avoid redraw if the staff/beats aren't visible...
				ww.renderstate.changed |= SCALE
				ww.publish(ev)
			}
		}()
		selxn := make(chan interface{})
		G.plumb.selection.Sub(&sc, selxn)
		sc.InitQuantizer(selxn)
	}
	ww.renderstate.changed |= SCALE | LAYOUT
	ww.publish(sc)
}

func (ww *WaveWidget) VisibleFrameRange() (FrameN, FrameN) {
	w0 := ww.first_frame
	wN := w0 + FrameN(ww.frames_per_pixel) * FrameN(ww.rect.wave.Dx())
	return w0, wN
}

func (ww *WaveWidget) SetCursorByFrame(frame FrameN) {
	ww.cursorX = ww.PixelAtFrame(frame)
	ww.renderstate.changed |= CURSOR
	ww.publish(frame)
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

func (ww *WaveWidget) dragBeat(min, max FrameN, beat *score.BeatRef) DragFn {
	return func(pos image.Point, finished bool, moved bool) bool {
		f := ww.FrameAtPixel(pos.X)
		if f <= min || f >= max || !moved {
			return false
		}
		ww.score.MvBeat(beat, f)
		return true
	}
}

func (ww *WaveWidget) selectDrag(anchor FrameN, snap bool) DragFn {
	return func(pos image.Point, finished bool, moved bool)bool {
		if !moved {
			return false
		}
		min := ww.FrameAtPixel(pos.X)
		max := anchor
		if max < min {
			min, max = max, min
		}
		if snap {
			ww.SelectAudioSnapToBeats(min, max)
		} else {
			ww.SelectAudio(FrameRange{min, max})
		}
		return true
	}
}

func (ww *WaveWidget) selectedNotes() []score.StaffNote {
	notes := make([]score.StaffNote, 0, len(ww.notesel))
	for note, nstaff := range ww.notesel {
		notes = append(notes, score.StaffNote{nstaff, note})
	}
	return notes
}

func (ww *WaveWidget) noteDrag(staff *score.Staff, note *score.Note) DragFn {
	sc := ww.score
	return func(pos image.Point, finished bool, moved bool)bool {
		prospect := ww.noteAtPixel(staff, pos)
		if prospect == nil {
			return false
		}
		Δpitch := staff.PitchForLine(prospect.delta) - note.Pitch
		Δbeat := prospect.beatf - sc.Beatf(note)
		_, selected := ww.notesel[note]
		if finished {
			if moved {
				if selected {
					sc.MvNotes(Δpitch, Δbeat, ww.selectedNotes()...)
				} else {
					sc.MvNotes(Δpitch, Δbeat, score.StaffNote{staff, note})
				}
			} else {
				/* regular click */
				_, selected := ww.notesel[note]
				if !selected {
					ww.notesel[note] = staff
				} else {
					delete(ww.notesel, note)
				}
				ww.renderstate.changed |= SCALE
				ww.publish(ww.notesel)
			}
		} else {
			if selected {
				ww.getMouseState(pos).ndelta = &noteDrag{Δpitch, Δbeat}
			} else {
				ww.getMouseState(pos).note = prospect
			}
			ww.renderstate.changed |= SCALE
			ww.publish(prospect)
		}
		return true
	}}

func (ww *WaveWidget) dragState(mouse image.Point) (DragFn, wde.Cursor) {
	rng := FrameRange{ww.FrameAtPixel(mouse.X - yspacing*2), ww.FrameAtPixel(mouse.X + yspacing*2)}
	sc := ww.score
	for staff, rect := range ww.rect.staves {
		if staff.Minimised || !mouse.In(rect) {
			continue
		}
		mid := rect.Min.Y + rect.Dy() / 2
		next := sc.Iter(rng, staff)
		var sn score.StaffNote
		for next != nil {
			sn, next = next()
			frame, _ := sc.ToFrame(sc.Beatf(sn.Note))
			x := ww.PixelAtFrame(frame)
			delta, _ := staff.LineForPitch(sn.Note.Pitch)
			y := mid - (yspacing / 2) * (delta)
			r := padPt(image.Pt(x, y), yspacing / 2, yspacing / 2)
			// XXX would be good to target the closest note instead of the first
			if mouse.In(r) {
				return ww.noteDrag(staff, sn.Note), wde.GrabHoverCursor
			}
		}
	}

	// TODO ignore beat grabs when sufficiently zoomed out
	if sc != nil {
		beat := sc.NearestBeat(ww.FrameAtPixel(mouse.X))
		if beat != nil {
			i := sc.BeatIndex(beat)
			x := ww.PixelAtFrame(beat.Frame())
			y0 := ww.rect.wave.Min.Y
			r := image.Rect(x, y0, x + 1, y0 + beatIncursion)
			if mouse.In(padRect(r, 2, 0)) {
				beats := sc.Beats()
				min, max := FrameN(0), ww.NFrames()
				if i + 1 < len(beats) {
					max = beats[i + 1].Frame()
				}
				if i > 0 {
					min = beats[i - 1].Frame()
				}
				return ww.dragBeat(min, max, beat), wde.ResizeEWCursor
			}
		}
	}

	snap := ww.score != nil && len(ww.score.Beats()) > 0 && (mouse.Y - ww.r.Min.Y < 4 * ww.r.Dy() / 5)
	if mouse.In(padRect(vrect(ww.r, ww.PixelAtFrame(ww.selection.MinFrame())), 2, 0)) {
		return ww.selectDrag(ww.selection.MaxFrame(), snap), wde.ResizeWCursor
	}
	if mouse.In(padRect(vrect(ww.r, ww.PixelAtFrame(ww.selection.MaxFrame())), 2, 0)) {
		return ww.selectDrag(ww.selection.MinFrame(), snap), wde.ResizeECursor
	}

	/* if we're not dragging anything in particular, drag to select */
	if mouse.In(ww.rect.wave) {
		return ww.selectDrag(ww.FrameAtPixel(mouse.X), snap), wde.NormalCursor
	}
	return nil, wde.NormalCursor
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

func (ww *WaveWidget) LeftButtonDown(mouse image.Point) DragFn {
	indent := ww.rect.wave.Min.X - ww.r.Min.X
	for staff, rect := range ww.rect.staves {
		r := leftRect(rect, indent)
		if mouse.In(r) {
			layout := MixerLayout{}
			layout.calc(yspacing, r)
			if mouse.In(layout.instC) {
				G.instMenu.SetDefault(staff.Voice())
				reply := G.instMenu.Popup(ww.r, ww.refresh, mouse)
				go func() {
					item := <-reply
					id, ok := item.(int)
					if item != nil && ok {
						staff.SetVoice(id)
					}
				}()
				return G.instMenu.Drag
			} else if mouse.In(layout.volS) {
				return func(pos image.Point, finished bool, moved bool)bool {
					if (moved || finished) && pos.In(layout.volS) {
						α := float64(pos.Y - layout.volS.Min.Y) / float64(layout.volS.Dy())
						vel := 127 - int(127.0 * α + 0.5)
						staff.SetVelocity(vel)
						return true
					}
					return false
				}
			}
		}
	}
	s := ww.getMouseState(mouse)
	return s.dragFn
}

func (n *noteProspect) Eq(n2 *noteProspect) bool {
	return n.staff == n2.staff && n.delta == n2.delta && n.beatf == n2.beatf
}

func (ww *WaveWidget) MouseMoved(mousePos image.Point) wde.Cursor {
	orig := ww.mouse.state
	s := ww.getMouseState(mousePos)
	if s.note != nil && (orig == nil || orig.note == nil || !s.note.Eq(orig.note)) {
		ww.renderstate.changed |= SCALE
		ww.publish(mousePos)
	}
	if !audio.IsPlaying() && ww.cursorX != mousePos.X && mousePos.X > ww.rect.wave.Min.X {
		ww.cursorX = mousePos.X
		ww.renderstate.changed |= CURSOR
		ww.publish(ww.cursorX)
	}
	return s.cursor
}

func (ww *WaveWidget) mkNote(prospect *noteProspect, dur *big.Rat) *score.Note {
	beat, offset := ww.score.Quantize(prospect.beatf)
	return &score.Note{prospect.staff.PitchForLine(prospect.delta), dur, beat, offset}
}

func (ww *WaveWidget) LeftClick(mouse image.Point) {
	//indent := ww.rect.wave.Min.X - ww.r.Min.X
	if mouse.In(ww.rect.newStaffB) && ww.score != nil {
		ww.score.AddStaff(&score.TrebleClef)
		ww.renderstate.changed |= LAYOUT
		ww.publish(&score.TrebleClef)
		return
	}
	for staff, layout := range ww.rect.mixers {
		if mouse.In(layout.r) {
			if mouse.In(layout.muteB) {
				staff.ToggleMute()
			} else if mouse.In(layout.minmaxB) {
				staff.Minimised = !staff.Minimised
				ww.renderstate.changed |= LAYOUT
				ww.publish(&staff.Minimised)
			}
		}
	}
}

func (ww *WaveWidget) RightButtonDown(mouse image.Point) DragFn {
	s := ww.getMouseState(mouse)
	if s.note == nil || ww.score == nil {
		return nil
	}
	note := s.note
	reply := G.noteMenu.Popup(ww.r, ww.refresh, mouse)
	go func() {
		item := <-reply
		str, ok := item.(string)
		if item != nil && ok {
			var dur *big.Rat = new(big.Rat)
			dur.SetString(str)
			newNote := ww.mkNote(note, dur)
			note.staff.AddNote(newNote)
			Synth.Note(Synth.Inst(midi.InstPiano), newNote.Pitch, 120, 100 * time.Millisecond)
		}
	}()
	return G.noteMenu.Drag
}

func (ww *WaveWidget) RightClick(mouse image.Point) {
	if mouse.In(ww.rect.mixer) {
		if mouse.In(ww.rect.newStaffB) && ww.score != nil {
			ww.score.AddStaff(&score.BassClef)
			ww.renderstate.changed |= LAYOUT
			ww.publish(&score.BassClef)
			return
		}
		for staff, _ := range ww.rect.staves {
			layout := ww.rect.mixers[staff]
			if mouse.In(layout.minmaxB) {
				staff.Minimised = false
				for staff2, _ := range ww.rect.staves {
					if staff2 != staff {
						staff2.Minimised = true
					}
				}
				ww.renderstate.changed |= LAYOUT
				ww.publish(&staff.Minimised)
				return
			}
		}
	}
}

func (ww *WaveWidget) MiddleButtonDown(mouse image.Point) DragFn {
	prevX := mouse.X
	return func(pos image.Point, finished bool, moved bool)bool {
		if moved {
			ww.ScrollPixels(prevX - pos.X)
			prevX = pos.X
		}
		return true
	}
}

func (ww *WaveWidget) Scroll(amount float64) int {
	return ww.ScrollPixels(int(float64(ww.r.Dx()) * amount))
}

func (ww *WaveWidget) ScrollPixels(dx int) int {
	if ww.r.Empty() || ww.wav == nil {
		return 0
	}
	original := ww.first_frame
	shift := FrameN(dx * ww.frames_per_pixel)
	rbound := ww.NFrames() - FrameN((ww.r.Dx() + 1) * ww.frames_per_pixel)
	ww.first_frame += shift
	if ww.first_frame < 0 || rbound < 0 {
		ww.first_frame = 0
	} else if ww.first_frame > rbound {
		ww.first_frame = rbound
	}
	diff := int(ww.first_frame - original)
	if diff != 0 {
		ww.renderstate.changed |= WAV | CURSOR | VIEWPOS
		ww.mouse.state = nil
		ww.publish(ww.first_frame)
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
		dx := x - ww.rect.wave.Min.X
		ww.first_frame = frameAtMouse - FrameN(dx * fpp)
		ww.frames_per_pixel = fpp
		ww.renderstate.changed |= WAV | CURSOR | VIEWPOS
		ww.mouse.state = nil
		ww.publish(fpp)
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
	nsharps := score.KeySig(-99)
	if s.note != nil {
		beatf := s.note.beatf
		delta = s.note.delta
		beat, o := ww.score.Quantize(beatf)
		beati = ww.score.BeatIndex(beat)
		offset = o
		pitch = s.note.staff.PitchForLine(delta)
		delta2, _ = s.note.staff.LineForPitch(pitch)
		nsharps = ww.score.Key()
	}

	return fmt.Sprintf("line=%d (%d) pitch=%d %s pos=%d:%v %v", delta, delta2, pitch, midi.PitchName(pitch), beati, offset, nsharps)
}
