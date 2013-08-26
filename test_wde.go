package main

import (
	"github.com/neagix/Go-SDL/sdl"
	"github.com/neagix/Go-SDL/sdl/audio"
	"github.com/neagix/Go-SDL/sound"
	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"code.google.com/p/freetype-go/freetype"
	"io/ioutil"
	"image"
	"sync"
	"time"
	"log"
	_ "os"
)

var wg sync.WaitGroup
var wave *WaveWidget

var fc *freetype.Context

func event(events <-chan interface{}, redraw chan image.Rectangle, done chan bool) {
	scroll := func(amount float64) {
		if wave.Scroll(amount) != 0 {
			redraw <- image.Rect(0, 0, 0, 0)
		}
	}
	zoom := func(factor float64) {
		if wave.Zoom(factor) != 0.0 {
			redraw <- image.Rect(0, 0, 0, 0)
		}
	}
	var refreshTimer *time.Timer
	for ei := range events {
		switch e := ei.(type) {
		case wde.KeyTypedEvent:
			log.Println("typed", e.Key, e.Glyph, e.Chord)
			switch e.Key {
			case wde.KeyLeftArrow:
				scroll(-0.25)
			case wde.KeyRightArrow:
				scroll(0.25)
			case wde.KeyUpArrow:
				zoom(0.5)
			case wde.KeyDownArrow:
				zoom(2.0)
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

func drawstuff(w wde.Window, redraw chan image.Rectangle, done chan bool) {
	/* TODO restart drawing if a new event comes in? */
	for {
		select {
		case <-redraw:
			s := w.Screen()
			width, height := w.Size()
			wvR := image.Rect(0, int(0.2*float32(height)), width, int(0.8*float32(height)))
			wave.Draw(s, wvR)
			statusR := image.Rect(0, wvR.Max.Y, width, height)
			wave.DrawStatus(s, fc, statusR)
			w.FlushImage()
		case <-done:
			return
		}
	}
}

func initfont() *freetype.Context {
	filename := "/usr/lib/go/site/src/code.google.com/p/freetype-go/luxi-fonts/luxisr.ttf"
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}
	font, err := freetype.ParseFont(bytes)
	if err != nil {
		log.Fatal(err)
	}

	fc := freetype.NewContext()
	fc.SetDPI(72)
	fc.SetFont(font)
	fc.SetFontSize(12)
	return fc
}

func main() {
	if sdl.Init(sdl.INIT_EVERYTHING) != 0 {
		log.Fatal(sdl.GetError())
	}

	sound.Init()

	desiredSpec := audio.AudioSpec{Freq: 44100, Format: audio.AUDIO_S16SYS, Channels: 1, Samples: 4096}
	var obtainedSpec audio.AudioSpec

	if audio.OpenAudio(&desiredSpec, &obtainedSpec) != 0 {
		log.Fatal(sdl.GetError())
	}

	fmt := sound.AudioInfo{obtainedSpec.Format, obtainedSpec.Channels, uint32(obtainedSpec.Freq)}

	sample := sound.NewSampleFromFile("test.ogg", &fmt, 1024*1024)
	sample.Decode()
	wav := NewWaveform(sample.Buffer_int16(), uint(obtainedSpec.Freq))
	log.Println(len(wav.Samples))

	fc = initfont()

	dw, err := wde.NewWindow(400, 400)
	if err != nil {
		log.Fatal(err)
	}
	dw.SetTitle("WDE test")
	dw.SetSize(400, 400)
	dw.Show()

	wave = NewWaveWidget()
	wave.SetWaveform(&wav)

	wg.Add(1)
	redraw := make(chan image.Rectangle, 5)
	done := make(chan bool)
	go drawstuff(dw, redraw, done)
	go event(dw.EventChan(), redraw, done)

	redraw <- image.Rect(0,0,0,0)

	wg.Wait()
}
