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
	var bpmT BpmTracker
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
			if !IsPlaying() && mousePos.In(wave.Rect()) {
				wave.SetCursorByPixel(mousePos.Sub(wave.Rect().Min))
			}
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
				playToggle(doredraw)
				bpmT.Clear()
			case wde.KeyReturn:
				if s, playing := CurrentSample(); playing {
					t := wav.TimeAtFrame(wav.ToFrame(s))
					bpmT.Append(t)
					b := bpmT.Bpm()
					if b != 0.0 {
						bpm.SetBpm(b)
						wave.SetBpm(b)
						wave.SetBeatAnchor(bpmT.Hits[0])
						doredraw()
					}
				}
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

func playToggle(feedback func()) {
	if IsPlaying() {
		fmt.Println("stopping playback")
		StopPlayback()
		return
	}

	f0, fN := wave.GetSelectedFrameRange()
	fmt.Println("starting playback", f0, fN)

	if f0 == fN {
		/* TODO play whole song starting from cursor*/
		return
	}

	/* short crossfade to loop smoothly */
	nchan := FrameN(wav.Channels)
	frame0 := wav.Frames(f0, f0)
	frameN := wav.Frames(fN, fN)
	nfPad := FrameN(20)
	loopPad := make([]int16, nchan*(2*nfPad + 1))
	N := fN - f0 + 1
	for i := FrameN(0); i < nfPad; i++ {
		α := 1.0 - float64(i)/float64(nfPad)
		for j := FrameN(0); j < nchan; j++ {
			loopPad[nchan*i + j] = int16(float64(frameN[j]) * α)
			loopPad[nchan*(2*nfPad - i) + j] = int16(float64(frame0[j]) * α)
		}
	}

	padN := N + FrameN(len(loopPad))/nchan
	bufsiz := FrameN(4096) // number of frames per buffer
	/* sample feeding i/o thread */
	go func() {
		var buf []int16
		i := FrameN(0)
		for {
			if i < N {
				if i + bufsiz > N {
					buf = wav.Frames(f0 + i, fN)
				} else {
					buf = wav.Frames(f0 + i, f0 + i + bufsiz - 1)
				}
			} else if i < padN {
				buf = loopPad
			}
			if AppendAudio(buf) == -1 {
				break
			}
			i += FrameN(len(buf)) / nchan
			i %= padN
		}
	}()
	//TODO wait for ring buffer to fill up a bit before kicking off audio
	StartPlayback(SampleN(nchan*f0), SampleN(nchan*padN))
	/* gui feedback thread */
	go func() {
		for {
			s, playing := CurrentSample()
			if !playing {
				break
			}
			wave.SetCursorByFrame(wav.ToFrame(s))
			time.Sleep(33 * time.Millisecond)
			feedback()
		}
	}()
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
