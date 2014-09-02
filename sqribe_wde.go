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
)

func event(events <-chan interface{}, redraw chan image.Rectangle, done chan bool, wg *sync.WaitGroup) {
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
					drag, _ = G.ww.CursorIconAtPixel(e.Where)
				}
			}
		case wde.MouseUpEvent:
			switch (e.Which) {
			case wde.LeftButton:
				if e.Where.In(G.ww.Rect()) {
					if !dragged {
						G.ww.LeftClick(e.Where)
					} else {
						drag(e.Where, true)
					}
				}
			case wde.RightButton:
				if e.Where.In(G.ww.Rect()) && !dragged {
					G.ww.RightClick(e.Where)
				}
			}
		case wde.MouseDraggedEvent:
			dragged = true
			switch (e.Which) {
			case wde.LeftButton:
				if drag != nil {
					drag(e.Where, false)
				}
			}
		case wde.MouseMovedEvent:
			G.mouse.pt = e.Where
			if G.mouse.pt.In(G.ww.Rect()) {
				if !IsPlaying() {
					G.ww.SetCursorByPixel(e.Where)
				}
				_, cur := G.ww.CursorIconAtPixel(e.Where)
				G.mouse.cursor.Set(cur)
			} else {
				G.mouse.cursor.Set(NormalCursor)
			}
		case wde.KeyTypedEvent:
			log.Println("typed", e.Key, e.Glyph, e.Chord)
			switch e.Key {
			case wde.KeyLeftArrow:
				G.ww.Scroll(-0.25)
			case wde.KeyRightArrow:
				G.ww.Scroll(0.25)
			case wde.KeyUpArrow:
				G.ww.Zoom(0.5)
			case wde.KeyDownArrow:
				G.ww.Zoom(2.0)
			case wde.KeyF2:
				G.score.nsharps--
			case wde.KeyF3:
				G.score.nsharps++
			case wde.KeyPrior:
				G.mixer.waveBias += 0.1
				if G.mixer.waveBias >= 1.0 {
					G.mixer.waveBias = 0.9
				}
			case wde.KeyNext:
				G.mixer.waveBias -= 0.1
				if G.mixer.waveBias <= 0.0 {
					G.mixer.waveBias = 0.1
				}
			case wde.KeySpace:
				playToggle()
			case wde.KeyReturn:
				if s, playing := CurrentSample(); playing {
					G.score.AddBeat(G.wav.ToFrame(s))
				}
			case wde.KeyS:
				SaveState(G.audiofile)
			case wde.KeyT:
				G.mixer.metronome = !G.mixer.metronome
			case wde.KeyA:
				G.mixer.audio = !G.mixer.audio
			case wde.KeyM:
				G.mixer.midi = !G.mixer.midi
			}
		case wde.ResizeEvent:
			if refreshTimer != nil {
				refreshTimer.Stop()
			}
			refreshTimer = time.AfterFunc(50*time.Millisecond, func() {redraw <- image.Rect(0, 0, 0, 0)})
		case wde.CloseEvent:
			done <- true
			wg.Done()
			return
		}
	}
}

func drawstatus(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	wstr := G.ww.Status()
	dx := G.mouse.pt.X - dst.Bounds().Min.X
	secs := G.ww.TimeAtCursor(dx)
	beat, _ := G.score.ToBeat(G.wav.FrameAtTime(secs))
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	G.font.luxi.Draw(dst, color.Black, r, fmt.Sprintf("%s %s  %f", wstr, secs, beat))
}

func drawstuff(w wde.Window, redraw chan image.Rectangle, done chan bool) {
	rate := time.Millisecond * 33 /* maximum refresh rate */
	lastframe := time.Now().Add(-rate)
	var refresh func()
	merged := 0
	for {
		select {
		case <-redraw:
			now := time.Now()
			nextframe := lastframe.Add(rate)
			if refresh != nil || now.Before(nextframe) {
				merged++
				if refresh == nil {
					refresh = func() {
						redraw <- image.Rect(0,0,0,0)
						refresh = nil
					}
					time.AfterFunc(nextframe.Sub(now), refresh)
				}
			} else {
				lastframe = now
				width, height := w.Size()
				r := image.Rect(0, 0, width, height)
				img := image.NewRGBA(r)
				wvR := image.Rect(0, int(0.2*float32(height)), width, int(0.8*float32(height) + 20))
				G.ww.Draw(img, wvR)

				statusR := image.Rect(0, wvR.Max.Y, width, height)
				drawstatus(img, statusR)

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

func InitWde(redraw chan image.Rectangle) (CursorCtl, *sync.WaitGroup) {
	dw, err := wde.NewWindow(400, 400)
	if err != nil {
		log.Fatal(err)
	}
	dw.SetTitle("Sqribe")
	dw.SetSize(400, 400)
	dw.Show()

	wg := sync.WaitGroup{}
	wg.Add(1)

	done := make(chan bool)
	go drawstuff(dw, redraw, done)
	go event(dw.EventChan(), redraw, done, &wg)

	return NewCursorCtl(dw), &wg
}
