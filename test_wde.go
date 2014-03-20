package main

import (
//	"github.com/neagix/Go-SDL/sdl"
	"github.com/neagix/Go-SDL/sdl/audio"
	"github.com/neagix/Go-SDL/sound"
	"github.com/sqweek/fluidsynth"
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
	"os"
)

var G struct {
	/* global state */
	audiofile string
	score Score
	wav *Waveform
	synth *fluidsynth.Synth

	/* ui stuff */
	ww *WaveWidget
	mouse struct {
		pt image.Point
		cursor CursorCtl
	}
	bpm BpmWidget
}

var wg sync.WaitGroup

func event(events <-chan interface{}, redraw chan image.Rectangle, done chan bool) {
	doredraw := func() {go func() {redraw <- image.Rect(0, 0, 0, 0)}()}
	var drag DragFn = nil
	var refreshTimer *time.Timer
	for ei := range events {
		switch e := ei.(type) {
		case wde.MouseDownEvent:
			switch (e.Which) {
			case wde.LeftButton:
				if e.Where.In(G.ww.Rect()) {
					drag, _ = G.ww.CursorIconAtPixel(e.Where)
				} else if e.Where.In(G.bpm.Rect()) {
					if newBpm := G.bpm.Hit(); newBpm != 0.0 {
						doredraw()
					}
				}
			}
		case wde.MouseDraggedEvent:
			switch (e.Which) {
			case wde.LeftButton:
				if drag != nil {
					drag(e.Where)
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
			case wde.KeySpace:
				playToggle()
			case wde.KeyReturn:
				if s, playing := CurrentSample(); playing {
					G.score.AddBeat(G.wav.ToFrame(s))
				}
			case wde.KeyS:
				SaveState(G.audiofile)
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

	f0, fN := G.ww.GetSelectedFrameRange()

	if f0 == fN {
		fN = G.wav.ToFrame(G.wav.NSamples) - 1
		f0 = G.ww.FrameAtCursor()
	}
	fmt.Println("starting playback", f0, fN)

	/* short crossfade to loop smoothly */
	nchan := FrameN(G.wav.Channels)
	frame0 := G.wav.Frames(f0, f0)
	frameN := G.wav.Frames(fN, fN)
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
		on := false
		var buf []int16
		i := FrameN(0)
		for {
			if i < N {
				if i + bufsiz > N {
					buf = G.wav.Frames(f0 + i, fN)
				} else {
					buf = G.wav.Frames(f0 + i, f0 + i + bufsiz - 1)
				}
			} else if i < padN {
				buf = loopPad
			}
			if on {
				G.synth.NoteOff(15, 89)
				on = false
			} else {
				b0, _ := G.score.ToBeat(f0 + i)
				bN, _ := G.score.ToBeat(f0 + i + bufsiz - 1)
				if int(b0) != int(bN) {
					G.synth.NoteOn(15, 77, 120)
					on = true
				}
			}
			nf := G.wav.ToFrame(SampleN(len(buf)))
			mbuf := G.synth.WriteFrames_int16(int(nf))
			for j := 0; j < len(buf); j++ {
				mbuf[j] += buf[j]
				mbuf[j] /= 2
			}
			if AppendAudio(mbuf) == -1 {
				break
			}
			i += nf
			i %= padN
		}
	}()
	//TODO wait for ring buffer to fill up a bit before kicking off audio
	StartPlayback(G.wav.ToSample(f0), G.wav.ToSample(padN))
	/* gui feedback thread */
	go func() {
		for {
			s, playing := CurrentSample()
			if !playing {
				break
			}
			G.ww.SetCursorByFrame(G.wav.ToFrame(s))
			time.Sleep(33 * time.Millisecond)
		}
	}()
}

func drawstatus(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	wstr := G.ww.Status()
	dx := G.mouse.pt.X - dst.Bounds().Min.X
	secs := G.ww.TimeAtCursor(dx)
	beat, _ := G.score.ToBeat(G.wav.FrameAtTime(secs))
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	RenderString(dst, color.Black, r, fmt.Sprintf("%s %s  %f", wstr, secs, beat))
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
				wvR := image.Rect(0, int(0.2*float32(height)), width, int(0.8*float32(height) + 20))
				G.ww.Draw(s, wvR)

				bpmR := image.Rect(width - 150, wvR.Max.Y, width, height)
				G.bpm.Draw(s, bpmR)

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

func open(filename string, fmt sound.AudioInfo) error {
	var err error
	if G.wav != nil {
		SaveState(G.audiofile)
	}
	G.wav, err = NewWaveform(filename, fmt)
	if err != nil {
		return err
	}
	G.audiofile = filename
	LoadState(filename)
	return nil
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

	err = open(*audioFile, actualFmt)
	if err != nil {
		log.Fatal(err)
	}

	err = FontInit()
	if err != nil {
		log.Fatal(err)
	}

	synth, err := SynthInit(int(sampleRate), "/d/synth/FluidR3_GM.sf2")
	if err != nil {
		log.Fatal(err)
	}
	G.synth = synth
	synth.ProgramChange(15, 115) // woodblock

	dw, err := wde.NewWindow(400, 400)
	if err != nil {
		log.Fatal(err)
	}
	dw.SetTitle("Sqribe")
	dw.SetSize(400, 400)
	dw.Show()
	G.mouse.cursor = NewCursorCtl(dw)

	redraw := make(chan image.Rectangle, 10)

	G.ww = NewWaveWidget(redraw)
	G.ww.SetWaveform(G.wav)
	G.score.Init()
	G.ww.SetScore(&G.score)

	G.bpm = BpmWidget{bpm: 120}

	wg.Add(1)
	done := make(chan bool)
	go drawstuff(dw, redraw, done)
	go event(dw.EventChan(), redraw, done)

	redraw <- image.Rect(0,0,0,0)

	wg.Wait()

	AudioShutdown()
	//XXX should avoid closing GUI if SaveState fails
	err = SaveState(G.audiofile)
	if err != nil {
		log.Println(err)
	}
	os.Remove(CacheFile())
}
