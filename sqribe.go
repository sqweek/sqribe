package main

import (
	"flag"
	"log"
	"os"
	"runtime"

	"sqweek.net/sqribe/audio"
	"sqweek.net/sqribe/fs"
	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/plumb"
	"sqweek.net/sqribe/score"
	"sqweek.net/sqribe/wave"

	. "sqweek.net/sqribe/core/data"
)

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
	instMenu MenuWidget
	noteMenu MenuWidget
	waveBias *SliderWidget
	font struct {
		luxi *Font
	}
	kb struct {
		shift bool
	}
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

	G.font.luxi = mustMkFont(fs.MustFind("luxisr.ttf"), 10)
	G.noteMenu = mkMenu(StringMenuOps{}, "1/16", "1/8", "1/4", "1/2", "1", "2", "3", "4")
	G.noteMenu.SetDefault("1")
	G.instMenu = mkMenu(StringMenuOps{toStr: func(item interface{})string {return midi.InstName(item.(int))}}, midi.InstPiano, midi.InstEPiano, midi.InstGuitar, midi.InstEGuitar, midi.InstViolin, midi.InstHarp, midi.InstVoice)

	Synth, err = SynthInit(audio.SampleRate, fs.MustFind("FluidR3_GM.sf2"))
	if err != nil {
		log.Fatal(err)
	}

	redraw := make(chan Widget, 10)

	G.ww = NewWaveWidget(redraw)

	wg := InitWde(redraw)

	// 1. audio callback thread
	// 2. ui event goroutine
	// 3. ui painting goroutine
	// 4. sample prefetch goroutine
	// 5. synth goroutine
	// 6. feedback goroutine
	// 7. quantizer
	// 8. io cache fetcher
	// 9. audio decoder
	runtime.GOMAXPROCS(6)

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
