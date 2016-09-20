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
	for staff, slayout := range ww.rect.staves() {
		if mouse.In(slayout.mix.r) {
			if mouse.In(slayout.mix.muteB) {
				toggle(&Mixer.For(staff).Muted)
				ww.changed(MIXER, ww)
			} else if mouse.In(slayout.mix.minmaxB) {
				toggle(&slayout.mix.Minimised)
				ww.changed(LAYOUT | SCALE, &slayout.mix.Minimised)
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
		staves := ww.rect.staves()
		for staff, slayout := range staves {
			if mouse.In(slayout.mix.minmaxB) {
				slayout.mix.Minimised = false
				for staff2, slayout2 := range staves {
					if staff2 != staff {
						slayout2.mix.Minimised = true
					}
				}
				ww.changed(LAYOUT | SCALE, &slayout.mix.Minimised)
				return
			} else if mouse.In(slayout.mix.muteB) {
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
		for staff, slayout := range ww.rect.staves() {
			if e.Where.In(slayout.mix.instC) {
				G.instMenu.SetDefault(Mixer.For(staff).Voice)
				reply := G.instMenu.Popup(slayout.mix.r, ww.refresh, e.Where)
				go func() {
					item := <-reply
					id, ok := item.(int)
					if item != nil && ok {
						Mixer.For(staff).Voice = id
					}
					ww.changed(MIXER, ww)
				}()
				return G.instMenu.Drag
			} else if e.Where.In(slayout.mix.volS) {
				return func(pos image.Point, finished bool, moved bool)bool {
					slider := slayout.mix.volS
					if (moved || finished) && pos.In(slider) {
						α := float64(pos.Y - slider.Min.Y) / float64(slider.Dy())
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
			ww.beatdrag = nil
			ww.changed(BEATS, beat)
			return false
		}
		if finished {
			ww.beatdrag = nil
			ww.score.MvBeat(beat, f)
		} else {
			newmap := make(map[*score.BeatRef]FrameN)
			newmap[beat] = f
			ww.beatdrag = newmap
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
			ww.toggleNotes(!(addToSel || G.kb.shift), score.StaffNote{staff, note})
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
	// XXX funny interaction with scrolling because mouse state gets cleared
	sc := ww.score
	addToSel := G.kb.shift
	fstart := ww.FrameAtPixel(start.X)
	return func(end image.Point, finished bool, moved bool)bool {
		r := image.Rect(ww.PixelAtFrame(fstart), start.Y, end.X, end.Y).Canon()
		if !finished {
			ww.getMouseState(end).rectSelect = &r
			ww.changed(SCALE, r)
		} else {
			notes := make([]score.StaffNote, 0, 8)
			var sn score.StaffNote
			next := sc.Iter(FrameRange{ww.FrameAtPixel(r.Min.X), ww.FrameAtPixel(r.Max.X)})
			for next != nil {
				sn, next = next()
				if slayout, ok := ww.rect.staves()[sn.Staff]; ok {
					y := ww.noteY(sn.Staff, sn.Note, centerPt(slayout.r).Y)
					if y >= r.Min.Y && y < r.Max.Y {
						notes = append(notes, sn)
					}
				}
			}
			ww.toggleNotes(!(addToSel || G.kb.shift), notes...)
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
	if staff, layout := ww.staffContaining(mouse); staff != nil {
		mid := layout.Mid()
		next := sc.Iter(rng, staff)
		var sn score.StaffNote
		var closest struct{d int; n *score.Note}
		for next != nil {
			sn, next = next()
			x := ww.noteX(sn.Note)
			y := ww.noteY(staff, sn.Note, mid)
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