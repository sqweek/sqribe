package main

import (
	"fmt"
	"github.com/sqweek/dialog"
	"image"
	"image/color"
	"image/draw"
	"sync"
	"time"

	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"sqweek.net/sqribe/audio"
	"sqweek.net/sqribe/log"
	"sqweek.net/sqribe/score"
)

func toggle(flag *bool) {
	*flag = !*flag
}

type DragState struct {
	fn DragFn
	moved bool
	button wde.Button
}

func event(win wde.Window, redraw chan Widget, done chan bool, wg *sync.WaitGroup) {
	openDlg := dialog.File().Title("sqribe - Open").Filter("Audio Files", "mp3", "ogg", "m4a", "wma", "mov", "mp4", "flv", "wmv").Filter("Sqribe Save", "sqs")
	exportDlg := dialog.File().Title("sqribe - Export to MusicXML").Filter("MXML Files", "xml", "mxl")
	events := win.EventChan()
	defer func() {
		done <- true
		wg.Done()
	}()
	var drag DragState
	var refreshTimer *time.Timer
	for ei := range events {
		switch e := ei.(type) {
		case wde.MouseDownEvent:
			if drag.button == 0 {
				drag.button = e.Which
				drag.moved = false
			}
			if e.Where.In(G.ww.Rect()) {
				if drag.fn == nil {
					drag.fn = G.ww.ButtonDown(e)
				} else {
					// hacky. wouldn't be necessary with a more sophisticated
					// API to start a drag action...
					switch (e.Which) {
					case wde.WheelUpButton:
						G.ww.Zoom(0.75)
					case wde.WheelDownButton:
						G.ww.Zoom(1.50)
					}
				}
			}
		case wde.MouseUpEvent:
			done := e.Which == drag.button
			if done {
				drag.button = 0
			}
			if drag.fn != nil {
				drag.fn(e.Where, done, drag.moved)
				if done {
					drag.fn = nil
				}
				continue
			}
			if drag.moved {
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
			drag.moved = true
			if drag.fn != nil {
				drag.fn(e.Where, false, true)
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
			log.UI.Println("typed", e.Key, e.Glyph, e.Chord)
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
			case e.Chord == "control+o":
				if playState == STOPPED {
					var err error
					var f string
					if err = save(); err == nil {
						// TODO should block rather than buffer input events during dialog
						if f, err = openDlg.Load(); err == nil {
							err = open(f)
						}
					}
					if err != nil && err != dialog.Cancelled {
						alert("%v", err)
					}
				}
			case e.Chord == "control+e":
				go func() {
					f, err := exportDlg.Save()
					if err == nil {
						err = ExportMXML(f)
					}
					if err != nil && err != dialog.Cancelled {
						alert("MXML export failed: %v", err)
					}
				}()
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
			case e.Key == wde.KeyF5:
				Synth.AdjustTuning(-10)
			case e.Key == wde.KeyF6:
				Synth.AdjustTuning(10)
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
				save()
			case e.Key == wde.KeyT:
				G.mixw.Toggle(&Mixer.MuteMetronome)
			case e.Key == wde.KeyA:
				G.mixw.Toggle(&Mixer.Wave.Muted)
			case e.Key == wde.KeyM:
				G.mixw.Toggle(&Mixer.Midi.Muted)
			case e.Key == wde.KeyQ:
				go G.score.QuantizeBeats()
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

func tuningStr() string {
	freq := Synth.TuningFreq()
	return fmt.Sprintf("A=%.4gHz", freq)
}

func drawstatus(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	G.font.luxi.Draw(dst, color.Black, r, fmt.Sprintf("%s  %v  %v", G.ww.Status(), quantizeStr(), tuningStr()))
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
		fatal(err)
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
