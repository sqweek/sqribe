package main

import (
//	"github.com/neagix/Go-SDL/sdl"
	SDL_audio "github.com/neagix/Go-SDL/sdl/audio"
	"github.com/neagix/Go-SDL/sound"
	"github.com/sqweek/fluidsynth"
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
)

var G struct {
	/* global state */
	audiofile string
	score score.Score
	wav *wave.Waveform
	synth *fluidsynth.Synth

	/* plumbing */
	plumb struct {
		selection *plumb.Port
		score *plumb.Port
	}

	/* ui stuff */
	ww *WaveWidget
	noteMenu MenuWidget
	mixer struct {
		metronome bool
		waveBias *SliderWidget
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

func playToggle() {
	if audio.IsPlaying() {
		fmt.Println("stopping playback")
		audio.Stop()
		return
	}

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
				G.synth.NoteOff(15, midi.PitchF6)
				on = false
			} else if G.mixer.metronome {
				b0, _ := G.score.ToBeat(f0 + i - 1)
				bN, _ := G.score.ToBeat(f0 + i + nf - 1)
				if int(b0) != int(bN) {
					G.synth.NoteOn(15, midi.PitchF6, 120)
					on = true
				}
			}
			/* user placed notes */
			for mi < len(mframes) && mframes[mi] <= f0 + i + nf {
				//fmt.Println(f0 + i, f0 + i + nf, mframes[mi])
				for ev, _ := mid[mframes[mi]]; ev != nil; ev = ev.Next {
					if ev.On {
						G.synth.NoteOn(0, int(ev.Pitch), 100)
					} else {
						G.synth.NoteOff(0, int(ev.Pitch))
					}
				}
				mi++
			}
			mbuf := G.synth.WriteFrames_int16(int(nf))
			if audio.Append(buf, mbuf) == -1 {
				break
			}
			i += nf
			if i >= padN {
				i = 0
				mi = 0
			}
		}
	}()
	//TODO wait for ring buffer to fill up a bit before kicking off audio
	audio.Play(G.wav.ToSample(f0), G.wav.ToSample(padN))
	/* gui feedback thread */
	go func() {
		for {
			s, playing := audio.PlayingSample()
			if !playing {
				break
			}
			G.ww.SetCursorByFrame(G.wav.ToFrame(s))
			time.Sleep(66 * time.Millisecond)
		}
	}()
}

func open(filename string, fmt sound.AudioInfo) error {
	var err error
	if G.wav != nil {
		SaveState(G.audiofile)
	}
	G.wav, err = wave.NewWaveform(filename, fmt)
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
	//sdl.Init(sdl.INIT_EVERYTHING)
	sound.Init()

	flag.Parse()

	channels, sampleRate, err := audio.Init()
	if err != nil {
		log.Fatal(err)
	}

	G.mixer.metronome = true

	G.plumb.selection = plumb.MkPort()
	G.plumb.score = plumb.MkPort()

	G.score.Init(G.plumb.score)

	actualFmt := sound.AudioInfo{SDL_audio.AUDIO_S16SYS, channels, uint32(sampleRate)}
	fmt.Println(actualFmt)

	G.font.luxi = mustMkFont("/usr/lib/go/site/src/code.google.com/p/freetype-go/luxi-fonts/luxisr.ttf", 10)
	G.noteMenu = mkStringMenu(4, "1/16", "1/8", "1/4", "1/2", "1", "2", "3", "4")

	synth, err := SynthInit(int(sampleRate), "/d/synth/FluidR3_GM.sf2")
	if err != nil {
		log.Fatal(err)
	}
	G.synth = synth
	synth.ProgramChange(15, midi.InstWoodblock)

	redraw := make(chan Widget, 10)

	G.ww = NewWaveWidget(redraw)

	wg := InitWde(redraw)

	audioFile := flag.Arg(0)
	if len(audioFile) > 0 {
		err = open(audioFile, actualFmt)
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
