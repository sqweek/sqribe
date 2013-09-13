package main

import (
	"github.com/neagix/Go-SDL/sdl"
	"github.com/neagix/Go-SDL/sdl/audio"
	"github.com/neagix/Go-SDL/sound"
	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"code.google.com/p/freetype-go/freetype"
	"io/ioutil"
	"image/draw"
	"image/color"
	"image"
	"sync"
	"time"
	"log"
	"fmt"
)

var wg sync.WaitGroup
var wav Waveform
var wave *WaveWidget
var mousePos image.Point

var fc *freetype.Context

func event(events <-chan interface{}, redraw chan image.Rectangle, done chan bool) {
	doredraw := func() {go func() {redraw <- image.Rect(0, 0, 0, 0)}()}
	scroll := func(amount float64) {
		if wave.Scroll(amount) != 0 {
			doredraw()
		}
	}
	zoom := func(factor float64) {
		if wave.Zoom(factor) != 0.0 {
			doredraw()
		}
	}
	var mouseDownPos image.Point
	var refreshTimer *time.Timer
	for ei := range events {
		switch e := ei.(type) {
		case wde.MouseDownEvent:
			mouseDownPos = e.Where
		case wde.MouseDraggedEvent:
			t1 := wave.TimeAtCursor(mouseDownPos.X)
			t2 := wave.TimeAtCursor(e.Where.X)
			if t1 < t2 {
				wave.SelectAudio(t1, t2)
			} else {
				wave.SelectAudio(t2, t1)
			}
			mousePos = e.Where
			doredraw()
		case wde.MouseMovedEvent:
			mousePos = e.Where
			doredraw()
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
			case wde.KeySpace:
				playToggle()
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

func playToggle() {
	if IsPlaying() {
		fmt.Println("stopping playback")
		StopPlayback()
		return
	}

	s0, sN := wave.GetSelectedSampleRange()
	fmt.Println("starting playback", s0, sN)

	/* short crossfade to loop smoothly */
	playSamples := wav.Samples[2*s0:2*sN]
	padlen := 20
	loopPad := make([]int16, 2*(2*padlen + 1))
	N := len(playSamples)/2
	for i := 0; i < padlen; i++ {
		alpha := 1.0 - float64(i)/float64(padlen)
		loopPad[2*i] = int16(float64(playSamples[2*(N-1)]) * alpha)
		loopPad[2*i + 1] = int16(float64(playSamples[2*(N-1) + 1]) * alpha)
		loopPad[2*(2*padlen - i)] = int16(float64(playSamples[0]) * alpha)
		loopPad[2*(2*padlen - i) + 1] = int16(float64(playSamples[1]) * alpha)
	}

	padN := N + len(loopPad)/2
	bufsiz := 4096
	go func() {
		var buf []int16
		i := 0
		for {
			buf = nil
			if i < N {
				if i + bufsiz > N {
					buf = playSamples[2*i:]
				} else {
					buf = playSamples[2*i:2*(i+bufsiz)]
				}
			} else if i < padN {
				iP := i - N
				if i + bufsiz > padN {
					buf = loopPad[2*iP:]
				} else {
					buf = loopPad[2*iP:2*(iP + bufsiz)]
				}
			}
			if AppendAudio(buf) == -1 {
				break
			}
			i += len(buf) / 2
			i %= padN
		}
	}()
	StartPlayback()
}

func drawstatus(dst draw.Image, fc *freetype.Context, r image.Rectangle) {
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	wstr := wave.Status()
	dx := mousePos.X - dst.Bounds().Min.X
	secs := wave.TimeAtCursor(dx)
	beat64 := wave.SixtyFourthAtTime(secs)
	measure := beat64/ 64
	beatInMeasure := beat64 % 64
	fc.SetDst(dst)
	fc.SetSrc(image.Black)
	fc.SetClip(r)
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	fc.DrawString(fmt.Sprintf("%s %f  %d:%d", wstr, secs, measure, beatInMeasure), freetype.Pt(r.Min.X + 10, r.Min.Y + 10))
}

func drawstuff(w wde.Window, redraw chan image.Rectangle, done chan bool) {
	rate := time.Millisecond * 33 /* maximum refresh rate */
	lastframe := time.Now().Add(-rate)
	var refresh *time.Timer
	merged := 0
	for {
		select {
		case <-redraw:
			now := time.Now()
			nextframe := lastframe.Add(rate)
			if refresh != nil || now.Before(nextframe) {
				merged++
				if refresh == nil {
					refresh = time.AfterFunc(nextframe.Sub(now), func() { redraw <- image.Rect(0,0,0,0); refresh = nil })
				}
			} else {
				lastframe = now
				s := w.Screen()
				width, height := w.Size()
				wvR := image.Rect(0, int(0.2*float32(height)), width, int(0.8*float32(height)))
				wave.Draw(s, wvR)
				statusR := image.Rect(0, wvR.Max.Y, width, height)
				drawstatus(s, fc, statusR)
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
	obtainedSpec, err := AudioInit(&desiredSpec)
	if err != nil {
		log.Fatal(err)
	}

	actualFmt := sound.AudioInfo{obtainedSpec.Format, obtainedSpec.Channels, uint32(obtainedSpec.Freq)}
	fmt.Println(actualFmt)

	sample := sound.NewSampleFromFile("test.ogg", &actualFmt, 1024*1024)
	sample.Decode()
	wav = NewWaveform(sample.Buffer_int16(), uint(obtainedSpec.Freq))
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
	redraw := make(chan image.Rectangle, 10)
	done := make(chan bool)
	go drawstuff(dw, redraw, done)
	go event(dw.EventChan(), redraw, done)

	redraw <- image.Rect(0,0,0,0)

	wg.Wait()
}
