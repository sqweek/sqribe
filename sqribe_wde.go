package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"sync"
	"time"

	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"sqweek.net/sqribe/audio"
	"sqweek.net/sqribe/score"
)

func toggle(flag *bool) {
	*flag = !*flag
}

func event(win wde.Window, redraw chan Widget, done chan bool, wg *sync.WaitGroup) {
	events := win.EventChan()
	defer func() {
		done <- true
		wg.Done()
	}()
	var drag DragFn = nil
	var dragged bool = false
	var refreshTimer *time.Timer
	for ei := range events {
		switch e := ei.(type) {
		case wde.MouseDownEvent:
			dragged = false
			switch (e.Which) {
			case wde.LeftButton:
				if e.Where.In(G.ww.Rect()) {
					drag = G.ww.LeftButtonDown(e.Where)
				}
			case wde.RightButton:
				if e.Where.In(G.ww.Rect()) {
					drag = G.ww.RightButtonDown(e.Where)
				}
			case wde.MiddleButton:
				if e.Where.In(G.ww.Rect()) {
					drag = G.ww.MiddleButtonDown(e.Where)
				}
			case wde.WheelUpButton:
				G.ww.Zoom(0.75)
			case wde.WheelDownButton:
				G.ww.Zoom(1.50)
			}
		case wde.MouseUpEvent:
			if drag != nil {
				drag(e.Where, true, dragged)
				drag = nil
				continue
			}
			if dragged {
				/* prevent drags from being interpreted as a regular click */
				continue
			}
			switch (e.Which) {
			case wde.LeftButton:
				if e.Where.In(G.ww.Rect()) {
					G.ww.LeftClick(e.Where)
				} else if e.Where.In(G.mixw.Rect()) {
					G.mixw.LeftClick(e.Where)
				}
			case wde.RightButton:
				if e.Where.In(G.ww.Rect()) {
					G.ww.RightClick(e.Where)
				} else if e.Where.In(G.mixw.Rect()) {
					G.mixw.RightClick(e.Where)
				}
			}
		case wde.MouseDraggedEvent:
			dragged = true
			if drag != nil {
				drag(e.Where, false, true)
			}
		case wde.MouseMovedEvent:
			var cur wde.Cursor = wde.NormalCursor
			if e.Where.In(G.ww.Rect()) {
				cur = G.ww.MouseMoved(e.Where)
			}
			win.SetCursor(cur)
		case wde.KeyDownEvent:
			switch e.Key {
			case wde.KeyLeftShift, wde.KeyRightShift:
				G.kb.shift = true
			}
		case wde.KeyUpEvent:
			switch e.Key {
			case wde.KeyLeftShift, wde.KeyRightShift:
				G.kb.shift = false
			}
		case wde.KeyTypedEvent:
			log.Println("typed", e.Key, e.Glyph, e.Chord)
			switch {
			case e.Chord == "shift+left_arrow":
				G.ww.ShuntSel(-1)
			case e.Chord == "shift+right_arrow":
				G.ww.ShuntSel(1)
			case e.Chord == "shift+" + wde.KeyInsert, e.Chord == "control+v", e.Key == wde.KeyInsert:
				G.ww.SetPasteMode(!G.ww.PasteMode())
			case e.Chord == "shift+" + wde.KeyDelete, e.Chord == "control+x":
				G.ww.Snarf()
				G.score.RemoveNotes(G.ww.SelectedNotes()...)
				G.ww.SetPasteMode(true)
			case e.Chord == "control+c":
				G.ww.Snarf()
				G.ww.SetPasteMode(true)
			case e.Chord == "shift+prior":
				G.mixw.AdjustGain(&Mixer.Midi.Gain, 0.1)
			case e.Chord == "shift+next":
				G.mixw.AdjustGain(&Mixer.Midi.Gain, -0.1)
			case e.Key == wde.KeyEscape:
				G.ww.SetPasteMode(false)
			case e.Key == wde.KeyLeftArrow:
				G.ww.Scroll(-0.25)
			case e.Key == wde.KeyRightArrow:
				G.ww.Scroll(0.25)
			case e.Key == wde.KeyUpArrow:
				G.ww.Zoom(0.5)
			case e.Key == wde.KeyDownArrow:
				G.ww.Zoom(2.0)
			case e.Key == wde.KeyF2:
				G.score.KeyChange(-1)
			case e.Key == wde.KeyF3:
				G.score.KeyChange(1)
			case e.Key == wde.KeyPrior:
				G.mixw.AdjustGain(&Mixer.Wave.Gain, 0.1)
			case e.Key == wde.KeyNext:
				G.mixw.AdjustGain(&Mixer.Wave.Gain, -0.1)
			case e.Key == wde.KeySpace:
				playToggle()
			case e.Key == wde.KeyReturn:
				if f, playing := audio.PlayingFrame(); playing {
					G.score.AddBeat(f)
				}
			case e.Key == wde.KeyDelete:
				G.score.RemoveNotes(G.ww.SelectedNotes()...)
			case e.Key == wde.KeyS:
				SaveState(G.audiofile)
			case e.Key == wde.KeyT:
				G.mixw.Toggle(&Mixer.MuteMetronome)
			case e.Key == wde.KeyA:
				G.mixw.Toggle(&Mixer.Wave.Muted)
			case e.Key == wde.KeyM:
				G.mixw.Toggle(&Mixer.Midi.Muted)
			case e.Key == wde.KeyQ:
				go G.score.QuantizeBeats()
			case e.Key == wde.KeyX:
				err := ExportMXML("export.xml")
				if err != nil {
					log.Println("MXML export", err)
				}
			case e.Glyph == "#":
				G.score.MvNotes(1, &rZero, G.ww.SelectedNotes()...)
			case e.Glyph == "@":
				G.score.MvNotes(-1, &rZero, G.ww.SelectedNotes()...)
			case e.Chord == "shift+8":
				G.score.MvNotes(-12, &rZero, G.ww.SelectedNotes()...)
			case e.Key == wde.Key8:
				G.score.MvNotes(12, &rZero, G.ww.SelectedNotes()...)
			case e.Glyph == "%":
				rng := G.ww.SelectedTimeRange()
				if beats, ok := rng.(score.BeatRange); ok {
					G.score.RepeatNotes(beats)
				}
			}
		case wde.ResizeEvent:
			if refreshTimer != nil {
				refreshTimer.Stop()
			}
			refreshTimer = time.AfterFunc(50*time.Millisecond, func() {redraw <- nil})
		case wde.CloseEvent:
			return
		}
	}
}

/* rounds sub-second duration to nearest ms/μs/ns */
func niceDur(dur time.Duration) string {
	if dur >= time.Second {
		return fmt.Sprintf("%.2fs", dur.Seconds())
	}
	switch {
	case dur >= time.Millisecond:
		return fmt.Sprintf("%dms", int(dur / time.Millisecond))
	case dur >= time.Microsecond:
		return fmt.Sprintf("%dµs", int(dur / time.Microsecond))
	default:
		return fmt.Sprintf("%dns", int(dur))
	}
}

func quantizeStr() string {
	q := G.score.QuantizeBeatStat()
	if q.Nop() {
		return ""
	}
	bpm := 60.0 * float64(time.Second) / float64(G.wav.TimeAtFrame(q.AvgFramesPerBeat()))
	errd := G.wav.TimeAtFrame(*q.Error)
	return fmt.Sprintf("%.1fbpm ±%v", bpm, niceDur(errd))
}

func drawstatus(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	G.font.luxi.Draw(dst, color.Black, r, fmt.Sprintf("%s  %v", G.ww.Status(), quantizeStr()))
}

func drawstuff(w wde.Window, redraw chan Widget, done chan bool) {
	rate := time.Millisecond * 25 /* maximum refresh rate */
	lastframe := time.Now().Add(-rate)
	var refresh func()
	merged := 0
	stale := make(map[Widget]struct{})
	for {
		select {
		case widget := <-redraw:
			if widget != nil {
				stale[widget] = struct{}{}
			}
			now := time.Now()
			nextframe := lastframe.Add(rate)
			if refresh != nil || now.Before(nextframe) {
				merged++
				if refresh == nil {
					refresh = func() {
						redraw <- widget
						refresh = nil
					}
					time.AfterFunc(nextframe.Sub(now), refresh)
				}
			} else {
				lastframe = now
				screen := w.Screen()
				r := screen.Bounds()
				width, height := r.Dx(), r.Dy()
				wvR := image.Rect(r.Min.X, r.Min.Y + 50, r.Max.X, r.Max.Y - 20)
				G.ww.Draw(screen, wvR)

				mixR := image.Rect(width - 90, wvR.Min.Y - 50, width, wvR.Min.Y)
				G.mixw.Draw(screen, mixR)

				statusR := image.Rect(0, wvR.Max.Y, width, height)
				statusI := image.NewRGBA(statusR)
				drawstatus(statusI, statusR)
				screen.CopyRGBA(statusI, statusR)

				if !G.noteMenu.Rect().Empty() {
					G.noteMenu.Draw(screen, G.noteMenu.Rect())
				}
				if !G.instMenu.Rect().Empty() {
					G.instMenu.Draw(screen, G.instMenu.Rect())
				}
				w.FlushImage()
				//log.Println("redraw took ", time.Now().Sub(lastframe), "  merged: ", merged)
				merged = 0
				lastframe = time.Now()
				for k, _ := range stale {
					delete(stale, k)
				}
			}
		case <-done:
			return
		}
	}
}

func InitWde(redraw chan Widget) *sync.WaitGroup {
	dw, err := wde.NewWindow(800, 400)
	if err != nil {
		log.Fatal(err)
	}
	dw.SetTitle("Sqribe")
	dw.SetSize(800, 400)
	dw.Show()

	wg := sync.WaitGroup{}
	wg.Add(1)

	done := make(chan bool)

	go drawstuff(dw, redraw, done)
	go event(dw, redraw, done, &wg)

	return &wg
}
