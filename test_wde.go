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
	"sort"
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
	mixer struct {
		metronome bool
		audio bool
		midi bool
		waveBias float64
	}
}

var wg sync.WaitGroup

func event(events <-chan interface{}, redraw chan image.Rectangle, done chan bool) {
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

type MidiEv struct {
	Pitch uint8
	On bool
	Next *MidiEv
}

func addEv(midi map[FrameN]*MidiEv, frame FrameN, ev MidiEv) {
	prev, ok := midi[frame]
	if ok {
		prev.Next = &ev
	} else {
		midi[frame] = &ev
	}
}

type FrameSlice []FrameN

func (f FrameSlice) Len() int {
	return len(f)
}

func (f FrameSlice) Less(i, j int) bool {
	return f[i] < f[j]
}

func (f FrameSlice) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
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
	N := fN - f0 + 1
	// pad to nearest 64th frame, minimum 20 frames
	nfPad := 19 + (64 - (N + 19) % 64)
	loopPad := make([]int16, nchan*(2*nfPad + 1))
	for i := FrameN(0); i < nfPad; i++ {
		α := 1.0 - float64(i)/float64(nfPad)
		for j := FrameN(0); j < nchan; j++ {
			loopPad[nchan*i + j] = int16(float64(frameN[j]) * α)
			loopPad[nchan*(2*nfPad - i) + j] = int16(float64(frame0[j]) * α)
		}
	}

	midi := make(map[FrameN]*MidiEv)
	for _, note := range(G.score.notes) {
		start, _ := G.score.ToFrame(note.Beatf())
		end, _ := G.score.ToFrame(note.Beatf() + note.Durf())
		if end <= f0 || start >= fN {
			continue
		}
		if start < f0 {
			start = f0
		}
		if end > fN {
			end = fN
		}
		addEv(midi, start, MidiEv{note.Pitch, true, nil})
		addEv(midi, end, MidiEv{note.Pitch, false, nil})
	}
	mframes := make([]FrameN, len(midi))
	i := 0
	for f, _ := range(midi) {
		mframes[i] = f
		for ev, _ := midi[f]; ev != nil; ev = ev.Next {
			fmt.Println(f, ev)
		}
		i++
	}
	sort.Sort(FrameSlice(mframes))

	padN := N + FrameN(len(loopPad))/nchan
	bufsiz := FrameN(64) // number of frames per buffer
	/* sample feeding i/o thread */
	go func() {
		on := false
		var buf []int16
		mi := 0
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
			nf := G.wav.ToFrame(SampleN(len(buf)))
			/* metronome */
			if on {
				G.synth.NoteOff(15, 77)
				on = false
			} else if G.mixer.metronome {
				b0, _ := G.score.ToBeat(f0 + i - 1)
				bN, _ := G.score.ToBeat(f0 + i + nf - 1)
				if int(b0) != int(bN) {
					G.synth.NoteOn(15, 77, 120)
					on = true
				}
			}
			/* user placed notes */
			for mi < len(mframes) && mframes[mi] <= f0 + i + nf {
				//fmt.Println(f0 + i, f0 + i + nf, mframes[mi])
				for ev, _ := midi[mframes[mi]]; ev != nil; ev = ev.Next {
					if ev.On {
						G.synth.NoteOn(0, int(ev.Pitch), 100)
					} else {
						G.synth.NoteOff(0, int(ev.Pitch))
					}
				}
				mi++
			}
			mbuf := G.synth.WriteFrames_int16(int(nf))
			if AppendAudio(buf, mbuf) == -1 {
				break
			}
			i += nf
			if i > padN {
				i = 0
				mi = 0
			}
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
	err = LoadState(filename)
	if err != nil {
		log.Println(err)
	}
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

	G.mixer.waveBias = 0.5
	G.mixer.metronome = true
	G.mixer.audio = true
	G.mixer.midi = true

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
