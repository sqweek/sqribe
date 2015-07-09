package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"sync"
	"time"

	"github.com/sqweek/go.wde"
	_ "github.com/sqweek/go.wde/init"
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
				} else if e.Where.In(G.waveBias.Rect()) {
					G.waveBias.LeftClick(e.Where)
				}
			case wde.RightButton:
				if e.Where.In(G.ww.Rect()) {
					G.ww.RightClick(e.Where)
				} else if e.Where.In(G.waveBias.Rect()) {
					G.waveBias.RightClick(e.Where)
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
		case wde.KeyTypedEvent:
			log.Println("typed", e.Key, e.Glyph, e.Chord)
			chord := true
			switch e.Chord {
			case "shift+left_arrow":
				G.ww.ShuntSel(-1)
			case "shift+right_arrow":
				G.ww.ShuntSel(1)
			case "shift+" + wde.KeyInsert, "ctrl+v":
				G.ww.SetPasteMode(!G.ww.PasteMode())
			case "shift+" + wde.KeyDelete, "ctrl+x":
				G.ww.Snarf()
				G.score.RemoveNotes(G.ww.SelectedNotes()...)
				G.ww.SetPasteMode(true)
			case "control+c":
				G.ww.Snarf()
				G.ww.SetPasteMode(true)
			default:
				chord = false
			}
			if chord {
				continue
			}
			switch e.Key {
			case wde.KeyEscape:
				G.ww.SetPasteMode(false)
			case wde.KeyLeftArrow:
				G.ww.Scroll(-0.25)
			case wde.KeyRightArrow:
				G.ww.Scroll(0.25)
			case wde.KeyUpArrow:
				G.ww.Zoom(0.5)
			case wde.KeyDownArrow:
				G.ww.Zoom(2.0)
			case wde.KeyF2:
				G.score.KeyChange(-1)
			case wde.KeyF3:
				G.score.KeyChange(1)
			case wde.KeyPrior:
				Mixer.Bias.Shunt(0.1)
			case wde.KeyNext:
				Mixer.Bias.Shunt(-0.1)
			case wde.KeySpace:
				playToggle()
			case wde.KeyReturn:
				if f, playing := audio.PlayingFrame(); playing {
					G.score.AddBeat(f)
				}
			case wde.KeyDelete:
				G.score.RemoveNotes(G.ww.SelectedNotes()...)
			default:
				switch e.Glyph {
				case "#":
					G.score.MvNotes(1, 0, G.ww.SelectedNotes()...)
				case "@":
					G.score.MvNotes(-1, 0, G.ww.SelectedNotes()...)
				case "%":
					rng := G.ww.SelectedTimeRange()
					if beats, ok := rng.(score.BeatRange); ok {
						G.score.RepeatNotes(beats)
					}
				case "s", "S":
					SaveState(G.audiofile)
				case "t", "T":
					toggle(&Mixer.MuteMetronome)
				case "a", "A":
					toggle(&Mixer.MuteWave)
				case "m", "M":
					toggle(&Mixer.MuteMidi)
				case "q", "Q":
					go G.score.QuantizeBeats()
				case "x", "X":
					err := ExportMXML("export.xml")
					if err != nil {
						log.Println("MXML export", err)
					}
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
		return dur.String()
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
	for {
		select {
		case widget := <-redraw:
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
				width, height := w.Size()
				r := image.Rect(0, 0, width, height)
				img := image.NewRGBA(r)
				wvR := image.Rect(0, 30, width, height - 20)
				G.ww.Draw(img, wvR)

				mixR := image.Rect(width - 50, wvR.Min.Y - 15, width, wvR.Min.Y)
				G.waveBias.Draw(img, mixR)

				statusR := image.Rect(0, wvR.Max.Y, width, height)
				drawstatus(img, statusR)

				if !G.noteMenu.Rect().Empty() {
					G.noteMenu.Draw(img, G.noteMenu.Rect())
				}
				if !G.instMenu.Rect().Empty() {
					G.instMenu.Draw(img, G.instMenu.Rect())
				}
				w.Screen().CopyRGBA(img, r)
				w.FlushImage()
				//log.Println("redraw took ", time.Now().Sub(lastframe), "  merged: ", merged)
				merged = 0
				lastframe = time.Now()
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

	G.waveBias = NewSlider(Mixer.Bias, false, redraw)

	go drawstuff(dw, redraw, done)
	go event(dw, redraw, done, &wg)

	return &wg
}
