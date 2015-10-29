package main

import (
	"image"
	"math/big"
	"time"

	"github.com/skelterjohn/go.wde"

	"sqweek.net/sqribe/midi"
)

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
						ww.changed(MIXER, ww)
					}
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
			newNote := ww.mkNote(note, dur)
			sc.AddNotes(note.staff, newNote)
			Synth.Note(Synth.Inst(midi.InstPiano), newNote.Pitch, 120, 100 * time.Millisecond)
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

