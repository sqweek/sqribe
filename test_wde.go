package main

import (
//	"github.com/neagix/Go-SDL/sdl"
	"github.com/neagix/Go-SDL/sdl/audio"
	"github.com/neagix/Go-SDL/sound"
	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"image/draw"
	"image/color"
	"image"
	"sync"
	"time"
	"flag"
	"log"
	"fmt"
)

var wg sync.WaitGroup
var wav *Waveform
var wave *WaveWidget
var mousePos image.Point
var bpm BpmWidget

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
	var dragOrigin image.Point
	var refreshTimer *time.Timer
	for ei := range events {
		switch e := ei.(type) {
		case wde.MouseDownEvent:
			switch (e.Which) {
			case wde.LeftButton:
				dragOrigin = e.Where
				if dragOrigin.In(bpm.Rect()) {
					if newBpm := bpm.Hit(); newBpm != 0.0 {
						wave.SetBpm(newBpm)
						doredraw()
					}
				}
			case wde.MiddleButton:
				wave.SetBeatAnchor(wave.TimeAtCursor(e.Where.X))
				doredraw()
			}
		case wde.MouseDraggedEvent:
			switch (e.Which) {
			case wde.LeftButton:
				r := wave.Rect()
				//log.Println(r, dragOrigin.Y - r.Min.Y, r.Dy() / 5)
				if dragOrigin.In(r) {
					t1 := wave.TimeAtCursor(dragOrigin.X)
					t2 := wave.TimeAtCursor(e.Where.X)
					if t1 > t2 {
						t1, t2 = t2, t1
					}
					if dragOrigin.Y - r.Min.Y < r.Dy() / 5 {
						wave.SelectAudioByTime(t1, t2)
					} else {
						wave.SelectAudioSnapToBars(t1, t2)
					}
					mousePos = e.Where
					doredraw()
				}
			}
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

	f0, fN := wave.GetSelectedFrameRange()
	fmt.Println("starting playback", f0, fN)

	/* short crossfade to loop smoothly */
	/* TODO have the go-routine that feeds audio wait for the samples
	 * instead of reading them all upfront */
	playSamples := wav.Samples(uint64(2*f0), uint64(2*fN))
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

func drawstatus(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	wstr := wave.Status()
	dx := mousePos.X - dst.Bounds().Min.X
	secs := wave.TimeAtCursor(dx)
	beat64 := wave.SixtyFourthAtTime(secs)
	measure := beat64/ 64
	beatInMeasure := beat64 % 64
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	RenderString(dst, color.Black, r, fmt.Sprintf("%s %s  %d:%d", wstr, secs, measure, beatInMeasure))
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
				s := w.Screen()
				width, height := w.Size()
				wvR := image.Rect(0, int(0.2*float32(height)), width, int(0.8*float32(height)))
				wave.Draw(s, wvR)

				bpmR := image.Rect(width - 150, wvR.Max.Y, width, height)
				bpm.Draw(s, bpmR)

				statusR := image.Rect(0, wvR.Max.Y, bpmR.Min.X, height)
				drawstatus(s, statusR)

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

var audioFile = flag.String("audio", "/d/music/Birds of Tokyo/Circles.mp3", "audio file")

func main() {
	//sdl.Init(sdl.INIT_EVERYTHING)
	sound.Init()

	channels, sampleRate, err := AudioInit()
	if err != nil {
		log.Fatal(err)
	}

	actualFmt := sound.AudioInfo{audio.AUDIO_S16SYS, channels, uint32(sampleRate)}
	fmt.Println(actualFmt)

	flag.Parse()

	wav, err = NewWaveform(*audioFile, actualFmt)
	if err != nil {
		log.Fatal(err)
	}

	err = FontInit()
	if err != nil {
		log.Fatal(err)
	}

	dw, err := wde.NewWindow(400, 400)
	if err != nil {
		log.Fatal(err)
	}
	dw.SetTitle("WDE test")
	dw.SetSize(400, 400)
	dw.Show()

	redraw := make(chan image.Rectangle, 10)

	wave = NewWaveWidget(redraw)
	wave.SetWaveform(wav)

	bpm = BpmWidget{bpm: 120}

	wg.Add(1)
	done := make(chan bool)
	go drawstuff(dw, redraw, done)
	go event(dw.EventChan(), redraw, done)

	redraw <- image.Rect(0,0,0,0)

	wg.Wait()

	AudioShutdown()
}
