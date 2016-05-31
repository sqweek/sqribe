package main

import (
	"image"
	"math/big"
	"time"

	"github.com/skelterjohn/go.wde"

	"github.com/sqweek/sqribe/audio"
	"github.com/sqweek/sqribe/midi"
	"github.com/sqweek/sqribe/score"

	. "github.com/sqweek/sqribe/core/types"
)

func (ww *WaveWidget) MouseMoved(mousePos image.Point) wde.Cursor {
	orig := ww.mouse.state
	s := ww.getMouseState(mousePos)
	if s.note != nil && (orig == nil || orig.note == nil || !s.note.Eq(orig.note)) {
		ww.changed(SCALE, mousePos)
	}
	if !audio.IsPlaying() && ww.cursorX != mousePos.X && mousePos.X > ww.rect.wave.Min.X {
		ww.cursorX = mousePos.X
		ww.changed(CURSOR, ww.cursorX)
	}
	return s.cursor
}

func (ww *WaveWidget) LeftClick(mouse image.Point) {
	if mouse.In(ww.rect.newStaffB) && ww.score != nil {
		ww.score.AddStaff(score.MkStaff("", &score.TrebleClef, ww.score.Key()))
		return
	}
	for staff, layout := range ww.rect.mixers {
		if mouse.In(layout.r) {
			if mouse.In(layout.muteB) {
				toggle(&Mixer.For(staff).Muted)
				ww.changed(MIXER, ww)
			} else if mouse.In(layout.minmaxB) {
				toggle(&layout.Minimised)
				ww.changed(LAYOUT | SCALE, &layout.Minimised)
			}
		}
	}
}

func (ww *WaveWidget) RightClick(mouse image.Point) {
	if mouse.In(ww.rect.newStaffB) && ww.score != nil {
		ww.score.AddStaff(score.MkStaff("", &score.BassClef, ww.score.Key()))
		return
	}
	if mouse.In(ww.rect.mixer) {
		for staff, _ := range ww.rect.staves {
			layout := ww.rect.mixers[staff]
			if mouse.In(layout.minmaxB) {
				layout.Minimised = false
				for staff2, layout2 := range ww.rect.mixers {
					if staff2 != staff {
						layout2.Minimised = true
					}
				}
				ww.changed(LAYOUT | SCALE, &layout.Minimised)
				return
			} else if mouse.In(layout.muteB) {
				Mixer.ToggleSolo(staff)
				ww.changed(MIXER, ww)
			}
		}
	}
	if ww.pasteMode {
		sc := ww.score
		if s := ww.getMouseState(mouse); s != nil && len(ww.snarf[s.note.staff]) > 0 {
			anchor := ww.snarf[s.note.staff][0]
			Δpitch := s.note.Δpitch(anchor)
			beat, offset := sc.Quantize(s.note.beatf)
			Δbeat := Δb(beat, offset, anchor.Beat, anchor.Offset)
			for staff, notes := range ww.snarf {
				mv := make([]*score.Note, 0, len(notes))
				for _, note := range notes {
					mv = append(mv, note.Dup().Mv(Δpitch, Δbeat))
				}
				sc.AddNotes(staff, mv...)
			}
			ww.pasteMode = false
		}
	}
}

func (ww *WaveWidget) ButtonDown(e wde.MouseDownEvent) DragFn {
	if e.Where.In(ww.rect.waveRulers) {
		switch e.Which {
		case wde.WheelUpButton:
			ww.Zoom(0.75)
			return nil
		case wde.WheelDownButton:
			ww.Zoom(1.50)
			return nil
		case wde.MiddleButton:
			return ww.scrollDrag(e.Where)
		case wde.RightButton:
			return ww.placeNoteDrag(e.Where)
		case wde.LeftButton:
			return ww.getMouseState(e.Where).dragFn
		}
	} else {
		for staff, layout := range ww.rect.mixers {
			if e.Where.In(layout.instC) {
				G.instMenu.SetDefault(Mixer.For(staff).Voice)
				reply := G.instMenu.Popup(layout.r, ww.refresh, e.Where)
				go func() {
					item := <-reply
					id, ok := item.(int)
					if item != nil && ok {
						Mixer.For(staff).Voice = id
					}
					ww.changed(MIXER, ww)
				}()
				return G.instMenu.Drag
			} else if e.Where.In(layout.volS) {
				return func(pos image.Point, finished bool, moved bool)bool {
					if (moved || finished) && pos.In(layout.volS) {
						α := float64(pos.Y - layout.volS.Min.Y) / float64(layout.volS.Dy())
						vel := 127 - int(127.0 * α + 0.5)
						Mixer.For(staff).Velocity = vel
						ww.changed(MIXER, ww)
						return true
					}
					return false
				}
			}
		}
	}
	return nil
}

func (ww *WaveWidget) placeNoteDrag(mouse image.Point) DragFn {
	s := ww.getMouseState(mouse)
	// XXX no way to exit pasteMode without pasting...
	sc := ww.score
	if s.note == nil || sc == nil || (ww.pasteMode && len(ww.snarf[s.note.staff]) > 0) {
		return nil
	}
	note := s.note
	reply := G.noteMenu.Popup(ww.Rect(), ww.refresh, mouse)
	go func() {
		item := <-reply
		str, ok := item.(string)
		if item != nil && ok {
			var dur *big.Rat = new(big.Rat)
			dur.SetString(str)
			n, exists := note.mkNote(sc, dur)
			if exists {
				n.Duration = dur
			}
			sc.AddNotes(note.staff, n)
			Synth.Note(Synth.Inst(midi.InstPiano), n.Pitch, 120, 100 * time.Millisecond)
		}
	}()
	return G.noteMenu.Drag
}

func (ww *WaveWidget) scrollDrag(mouse image.Point) DragFn {
	prevX := mouse.X
	return func(pos image.Point, finished bool, moved bool)bool {
		if moved {
			ww.ScrollPixels(prevX - pos.X)
			prevX = pos.X
		}
		return true
	}
}

func (ww *WaveWidget) beatDrag(beat *score.BeatRef) DragFn {
	prev, next := beat.Prev(), beat.Next()
	rng := ww.WaveRange()
	min, max := rng.MinFrame(), rng.MaxFrame()
	if next != nil {
		max = next.Frame()
	}
	if prev != nil {
		min = prev.Frame()
	}
	return func(pos image.Point, finished bool, moved bool) bool {
		if !moved {
			return false
		}
		f := ww.FrameAtPixel(pos.X)
		if f <= min || f >= max || !pos.In(ww.rect.waveRulers) {
			delete(ww.beatdrag, beat)
			ww.changed(BEATS, beat)
			return false
		}
		if finished {
			delete(ww.beatdrag, beat)
			ww.score.MvBeat(beat, f)
		} else {
			ww.beatdrag[beat] = f
			ww.changed(BEATS, beat)
		}
		return true
	}
}

func (ww *WaveWidget) timeSelectDrag(anchor FrameN, snap bool) DragFn {
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

func (ww *WaveWidget) noteDrag(staff *score.Staff, note *score.Note) DragFn {
	sc := ww.score
	addToSel := G.kb.shift
	return func(pos image.Point, finished bool, moved bool)bool {
		if finished && !moved {
			/* regular click */
			ww.selectNotes(!(addToSel || G.kb.shift), score.StaffNote{staff, note})
			ww.changed(SCALE, ww.notesel)
			return true
		}
		prospect := ww.noteAtPixel(staff, pos)
		if prospect == nil {
			return false
		}
		Δpitch := prospect.Δpitch(note)
		beat, offset := sc.Quantize(prospect.beatf)
		Δbeat := Δb(beat, offset, note.Beat, note.Offset)
		_, selected := ww.notesel[note]
		if finished {
			/* `moved` must be true */
			ww.mouse.state = nil
			if selected {
				sc.MvNotes(Δpitch, Δbeat, ww.SelectedNotes()...)
			} else {
				sc.MvNotes(Δpitch, Δbeat, score.StaffNote{staff, note})
			}
		} else {
			if selected {
				ww.getMouseState(pos).ndelta = &noteDrag{Δpitch, Δbeat}
			} else {
				ww.getMouseState(pos).note = prospect
			}
			ww.changed(SCALE, prospect)
		}
		return true
	}
}

func (ww *WaveWidget) noteSelectDrag(start image.Point) DragFn {
	// XXX funny interaction with scrolling because we hold on to pixel values
	sc := ww.score
	addToSel := G.kb.shift
	return func(end image.Point, finished bool, moved bool)bool {
		r := image.Rectangle{start, end}.Canon()
		if !finished {
			ww.getMouseState(end).rectSelect = &r
			ww.changed(SCALE, r)
		} else {
			notes := make([]score.StaffNote, 0, 8)
			var sn score.StaffNote
			next := sc.Iter(ww.VisibleFrameRange())
			for next != nil {
				sn, next = next()
				dn := ww.dispNote(sn.Staff, sn.Note, centerPt(ww.rect.staves[sn.Staff]).Y)
				if dn.pt != nil && dn.pt.In(r) {
					notes = append(notes, sn)
				}
			}
			ww.selectNotes(!(addToSel || G.kb.shift), notes...)
			ww.getMouseState(end).rectSelect = nil
			ww.changed(SCALE, &ww.notesel)
		}
		return true
	}
}

func (ww *WaveWidget) dragState(mouse image.Point) (DragFn, wde.Cursor) {
	beath := 8
	grabw := 2
	sc := ww.score
	r := ww.Rect()
	bAxis, tAxis := mouse.In(ww.rect.beatAxis), mouse.In(ww.rect.timeAxis)
	snap := bAxis && sc != nil && sc.HasBeats()
	if bAxis || tAxis {
		if mouse.In(padRect(vrect(r, ww.PixelAtFrame(ww.selection.MinFrame())), grabw, 0)) {
			return ww.timeSelectDrag(ww.selection.MaxFrame(), snap), wde.ResizeWCursor
		} else if mouse.In(padRect(vrect(r, ww.PixelAtFrame(ww.selection.MaxFrame())), grabw, 0)) {
			return ww.timeSelectDrag(ww.selection.MinFrame(), snap), wde.ResizeECursor
		}
		return ww.timeSelectDrag(ww.FrameAtPixel(mouse.X), snap), wde.IBeamCursor
	}

	rng := FrameRange{ww.FrameAtPixel(mouse.X - yspacing*2), ww.FrameAtPixel(mouse.X + yspacing*2)}
	for staff, rect := range ww.rect.staves {
		if mix, ok := ww.rect.mixers[staff]; (ok && mix.Minimised) || !mouse.In(rect) {
			continue
		}
		mid := rect.Min.Y + rect.Dy() / 2
		next := sc.Iter(rng, staff)
		var sn score.StaffNote
		var closest struct{d int; n *score.Note}
		for next != nil {
			sn, next = next()
			frame, _ := sc.ToFrame(sc.Beatf(sn.Note))
			x := ww.PixelAtFrame(frame)
			delta, _ := staff.LineForPitch(sn.Note.Pitch)
			y := mid - (yspacing / 2) * (delta)
			d := sqdist(mouse.X, mouse.Y, x, y)
			if d < (yspacing*yspacing)/4 && (closest.n == nil || d < closest.d) {
				closest.d = d
				closest.n = sn.Note
			}
		}
		if closest.n != nil {
			return ww.noteDrag(staff, closest.n), wde.GrabHoverCursor
		}
	}

	// TODO ignore beat grabs when sufficiently zoomed out
	if sc != nil && mouse.Y <= ww.rect.wave.Min.Y + beath {
		beat := sc.NearestBeat(ww.FrameAtPixel(mouse.X))
		if beat != nil {
			x := ww.PixelAtFrame(beat.Frame())
			if x - grabw <= mouse.X && mouse.X <= x + grabw {
				return ww.beatDrag(beat), wde.ResizeEWCursor
			}
		}
	}

	if mouse.In(ww.rect.wave) {
		return ww.noteSelectDrag(mouse), wde.NormalCursor
	}

	return nil, wde.NormalCursor
}
