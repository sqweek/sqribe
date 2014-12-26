package main

import (
	"sort"
	"time"
	"flag"
	"log"
	"fmt"
	"os"

	"sqweek.net/sqribe/audio"
	"sqweek.net/sqribe/fs"
	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/plumb"
	"sqweek.net/sqribe/score"
	"sqweek.net/sqribe/wave"

	. "sqweek.net/sqribe/core/types"
	. "sqweek.net/sqribe/core/data"
)

var Mixer struct {
	Bias *BoundFloat
	MuteWave, MuteMidi, MuteMetronome bool
}

var G struct {
	/* global state */
	audiofile string
	score score.Score
	wav *wave.Waveform

	/* plumbing */
	plumb struct {
		selection *plumb.Port
		score *plumb.Port
	}

	/* ui stuff */
	ww *WaveWidget
	noteMenu MenuWidget
	waveBias *SliderWidget
	mixer struct {
	}
	font struct {
		luxi *Font
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
		for ; prev.Next != nil; prev = prev.Next {}
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

const (
	STOPPED = iota
	PLAYING
	STOPPING
)

/* globally mutable state... that's not thinking with channels :S */
var playState int = STOPPED

func playToggle() {
	switch playState {
	case PLAYING:
		fmt.Println("stopping playback")
		playState = STOPPING
		return
	case STOPPING:
		return /* in transition; do nothing */
	}

	playState = PLAYING
	audio.Clear()
	rng := G.ww.GetSelectedTimeRange()
	f0, fN := rng.MinFrame(), rng.MaxFrame()

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
	loopPad := make([]int16, nchan*nfPad)
	for i := FrameN(0); i < nfPad; i++ {
		α := 1.0 - float64(i)/float64(nfPad)
		for j := FrameN(0); j < nchan; j++ {
			if α <= 0.5 {
				loopPad[nchan*i + j] = int16(float64(frame0[j]) * 2 * (0.5 - α))
			} else {
				loopPad[nchan*i + j] = int16(float64(frameN[j]) * 2 * (α - 0.5))
			}
		}
	}

	mid := make(map[FrameN]*MidiEv)
	notes := make(chan *score.Note, 5)
	go score.OrderNotes(&G.score, notes)
	for note := range(notes) {
		start, _ := G.score.ToFrame(G.score.Beatf(note))
		end, _ := G.score.ToFrame(G.score.Beatf(note) + note.Durf())
		if end <= f0 || start >= fN {
			continue
		}
		if start < f0 {
			start = f0
		}
		if end > fN {
			end = fN
		}
		addEv(mid, start, MidiEv{note.Pitch, true, nil})
		addEv(mid, end, MidiEv{note.Pitch, false, nil})
	}
	mframes := make([]FrameN, len(mid))
	i := 0
	for f, _ := range(mid) {
		mframes[i] = f
		for ev, _ := mid[f]; ev != nil; ev = ev.Next {
			//fmt.Println(f, ev)
		}
		i++
	}
	sort.Sort(FrameSlice(mframes))

	padN := N + G.wav.ToFrame(SampleN(len(loopPad)))
	/* wave sample prefetch thread */
	sampch := make(chan []int16, 10)
	go func() {
		bufsiz := FrameN(2048)
		var buf []int16
		i := FrameN(0)
		for playState == PLAYING {
			if i + bufsiz > N {
				wave := G.wav.Frames(f0 + i, fN)
				buf = make([]int16, len(wave) + len(loopPad))
				copy(buf, wave)
				copy(buf[len(wave):], loopPad)
			} else {
				buf = G.wav.Frames(f0 + i, f0 + i + bufsiz - 1)
			}
			nf := G.wav.ToFrame(SampleN(len(buf)))
			sampch <- buf
			i += nf
			if i >= padN {
				i = 0
			}
		}
		close(sampch)
	}()
	/* synth & sample feeding thread */
	go func() {
		woodblock := Synth.Inst(midi.InstWoodblock)
		piano := Synth.Inst(midi.InstPiano)
		on := false
		nf := FrameN(64)
		bufsiz := int(G.wav.ToSample(nf))
		mbuf := make([]int16, bufsiz)
		inbuf := []int16{}
		mi := 0
		i := FrameN(0)
		for playState == PLAYING {
			if len(inbuf) == 0 {
				inbuf = <-sampch
				if len(inbuf) < bufsiz || len(inbuf) % bufsiz != 0 {
					fmt.Println("prefetch samples sent in non-64 frame multiple", len(inbuf))
					playState = STOPPING
					break
				}
			}
			buf := inbuf[:bufsiz]
			inbuf = inbuf[bufsiz:]

			/* metronome */
			if on {
				Synth.NoteOff(woodblock, midi.PitchF6)
				on = false
			} else if !Mixer.MuteMetronome {
				b0, _ := G.score.ToBeat(f0 + i - 1)
				bN, _ := G.score.ToBeat(f0 + i + nf - 1)
				if int(b0) != int(bN) {
					Synth.NoteOn(woodblock, midi.PitchF6, 120)
					on = true
				}
			}
			/* user placed notes */
			for mi < len(mframes) && mframes[mi] <= f0 + i + nf {
				//fmt.Println(f0 + i, f0 + i + nf, mframes[mi])
				for ev, _ := mid[mframes[mi]]; ev != nil; ev = ev.Next {
					// TODO get channel for voice associated with staff
					if ev.On {
						Synth.NoteOn(piano, ev.Pitch, 100)
					} else {
						Synth.NoteOff(piano, ev.Pitch)
					}
				}
				mi++
			}
			Synth.WriteFrames(mbuf)
			α, β := 0.0, 0.0
			bias := Mixer.Bias.Value()
			if !Mixer.MuteWave {
				α = 0.5 + bias
			}
			if !Mixer.MuteMidi {
				β = 0.5 - bias
			}
			for j := 0; j < bufsiz; j++ {
				mbuf[j] = int16(α * float64(buf[j]) + β * float64(mbuf[j]))
			}
			audio.Append(mbuf)
			i += nf
			if i >= padN {
				i = 0
				mi = 0
			}
		}
		for _ = range(sampch) {
			// drain channel
		}
		fmt.Println("notifying portaudio")
		audio.Stop()
		playState = STOPPED
		fmt.Println("playback all stopped")
	}()
	//TODO wait for ring buffer to fill up a bit before kicking off audio
	audio.Play(G.wav.ToSample(f0), G.wav.ToSample(padN))
	/* gui feedback thread */
	go func() {
		for {
			s, playing := audio.PlayingSample()
			if !playing {
				if playState == PLAYING && s != 0 {
					/* we think we're playing, but the audio callback hasn't
					 * run for awhile. just stop. */
					fmt.Println("lost audio callback, stopping")
					playState = STOPPING
				}
				break
			}
			G.ww.SetCursorByFrame(G.wav.ToFrame(s))
			time.Sleep(66 * time.Millisecond)
		}
	}()
}

func open(filename string) error {
	var err error
	if G.wav != nil {
		SaveState(G.audiofile)
	}
	G.wav, err = wave.NewWaveform(filename)
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

func mustMkFont(filename string, size int) *Font {
	font, err := NewFont(filename, size)
	if err != nil {
		log.Fatal(err)
	}
	return font
}

func main() {
	flag.Parse()

	err := audio.Open()
	if err != nil {
		log.Fatal(err)
	}

	Mixer.Bias = MkBoundFloat(0, -0.5, 0.5, nil)

	G.plumb.selection = plumb.MkPort()
	G.plumb.score = plumb.MkPort()

	G.score.Init(G.plumb.score)

	fmt.Printf("audio opened with %d channels @ %d Hz\n", audio.Channels, audio.SampleRate)

	G.font.luxi = mustMkFont("/d/go/src/code.google.com/p/freetype-go/testdata/luxisr.ttf", 10)
	G.noteMenu = mkStringMenu(4, "1/16", "1/8", "1/4", "1/2", "1", "2", "3", "4")

	Synth, err = SynthInit(audio.SampleRate, "/d/synth/FluidR3_GM.sf2")
	if err != nil {
		log.Fatal(err)
	}

	redraw := make(chan Widget, 10)

	G.ww = NewWaveWidget(redraw)

	wg := InitWde(redraw)

	audioFile := flag.Arg(0)
	if len(audioFile) > 0 {
		err = open(audioFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	G.ww.SetWaveform(G.wav)
	G.ww.SetScore(&G.score)

	redraw <- nil

	wg.Wait()

	audio.Shutdown()
	//XXX should avoid closing GUI if SaveState fails
	err = SaveState(G.audiofile)
	if err != nil {
		log.Println(err)
	}
	os.Remove(fs.CacheFile())
}
