package main

import (
//	"github.com/neagix/Go-SDL/sdl"
	"github.com/neagix/Go-SDL/sdl/audio"
	"github.com/neagix/Go-SDL/sound"
	"github.com/sqweek/fluidsynth"
	"image"
	"sort"
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

	quantize struct {
		apply chan chan bool
		calc chan chan QuantizeBeats
	}

	/* plumbing */
	plumb struct {
		selection *PlumbPort
	}

	/* ui stuff */
	ww *WaveWidget
	mouse struct {
		pt image.Point
	}
	mixer struct {
		metronome bool
		audio bool
		midi bool
		waveBias float64
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

	channels, sampleRate, err := AudioInit()
	if err != nil {
		log.Fatal(err)
	}

	G.mixer.waveBias = 0.5
	G.mixer.metronome = true
	G.mixer.audio = true
	G.mixer.midi = true

	G.quantize.apply = make(chan chan bool)
	G.quantize.calc = make(chan chan QuantizeBeats)

	G.plumb.selection = MkPort()

	actualFmt := sound.AudioInfo{audio.AUDIO_S16SYS, channels, uint32(sampleRate)}
	fmt.Println(actualFmt)

	flag.Parse()

	err = open(*audioFile, actualFmt)
	if err != nil {
		log.Fatal(err)
	}

	G.font.luxi = mustMkFont("/usr/lib/go/site/src/code.google.com/p/freetype-go/luxi-fonts/luxisr.ttf", 10)

	synth, err := SynthInit(int(sampleRate), "/d/synth/FluidR3_GM.sf2")
	if err != nil {
		log.Fatal(err)
	}
	G.synth = synth
	synth.ProgramChange(15, 115) // woodblock

	redraw := make(chan image.Rectangle, 10)

	G.ww = NewWaveWidget(redraw)
	G.ww.SetWaveform(G.wav)
	G.score.Init()
	G.ww.SetScore(&G.score)

	wg := InitWde(redraw)

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
