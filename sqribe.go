package main

import (
	"flag"
	"fmt"
	"github.com/sqweek/dialog"
	"github.com/sqweek/fs"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/sqweek/sqribe/audio"
	"github.com/sqweek/sqribe/log"
	"github.com/sqweek/sqribe/midi"
	"github.com/sqweek/sqribe/plumb"
	"github.com/sqweek/sqribe/score"
	"github.com/sqweek/sqribe/wave"
)

var G struct {
	/* global state */
	files FileContext
	score *score.Score
	wav *wave.Waveform

	/* plumbing */
	plumb struct {
		selection *plumb.Port
		score *plumb.Port
	}

	/* ui stuff */
	ww *WaveWidget
	mixw *MixWidget
	instMenu MenuWidget
	noteMenu MenuWidget
	font struct {
		luxi *Font
	}
	kb struct {
		shift bool
	}
}

var ZeroTime time.Time

func open(filename string) error {
	files, s, err := Open(filename)
	if !files.Timestamp.IsZero() {
		// sanity check for correct state file
		hfile := s.Headers().String("Filename")
		if hfile != "" && hfile != files.Audio {
			return fmt.Errorf("found state (%s) for wrong audio file (got %s; wanted %s)", files.State, hfile, files.Audio)
		}
	}

	if err := os.MkdirAll(App.Cache, 0777); err != nil {
		return err
	}
	// TODO allow user to locate audio if it doesn't exist (eg. been moved)
	loadErr := make(chan error)
	wav, err := wave.NewWaveform(files.Audio, filepath.Join(App.Cache, *cachefile), loadErr)
	if err != nil {
		return err
	}
	go func() {
		_ = <-loadErr
		/* audio has finished loading, either successfully or unsuccessfully.
		 * some mp3s apparently contain stupid stuff like ID3 tags in the middle of
		 * the last data frame, so a user alert is not raised here...
		 * Just send some spurious change events now that waveform's NSamples is stable. */
		G.plumb.score.C <- score.BeatChanged{}
		G.plumb.score.C <- score.StaffChanged{}
	}()

	// point of no return; nothing errors after this and we transition to the new file
	s.Restore()
	G.files = files
	G.wav = wav
	old := G.ww.SetWaveform(wav)
	if old != nil {
		go old.Close()
	}
	return nil
}

func save() error {
	if G.files.Audio == "" {
		return nil // save with no audio loaded is a no-op
	}
	if !G.files.Timestamp.IsZero() {
		// user needs to be able to recover from these errors
		st, err := os.Stat(G.files.State)
		if err != nil && !os.IsNotExist(err) {
			return err // not a very clear error...
		}
		if st.ModTime() != G.files.Timestamp {
			return fmt.Errorf("file has changed since loading")
		}
	}
	s := CaptureState()
	if err := SaveState(&G.files, s); err != nil {
		return err
	}
	if st, err := os.Stat(G.files.State); err == nil {
		G.files.Timestamp = st.ModTime()
	} else {
		log.FS.Println("warning: couldn't retreive timestamp:", err)
		G.files.Timestamp = ZeroTime
	}
	return nil
}

func mustMkFont(filename string, size int) *Font {
	font, err := NewFont(filename, size)
	if err != nil {
		panic(err)
	}
	return font
}

var initialTime = flag.Duration("time", 0, "position initial view at this time (eg 1m32s)")
var profile = flag.String("prof", "", "write cpu profile to file")
var cachefile = flag.String("cache", "", "cache file name")

func alert(format string, args... interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("ERROR %s", msg)
	dialog.Message("%s", msg).Title("sqribe - error").Error()
}

func fatal(args... interface{}) {
	msg := fmt.Sprint(args...)
	log.Printf("FATAL %s", msg)
	dialog.Message("%s", msg).Title("sqribe - fatal error").Error()
	os.Exit(1)
}

func main() {
	flag.Parse()
	if err := fsinit("net.sqweek.sqribe", "sqribe"); err != nil {
		fatal(err)
	}
	if *cachefile == "" {
		main_parent()
	} else {
		wderun(main_child)
		//XXX should avoid closing GUI if save fails
		if err := save(); err != nil {
			log.FS.Println(err)
		}
	}
}

type TeeLogger struct {
	writers []io.Writer
	errs int
}

func (t *TeeLogger) Write(buf []byte) (n int, err error) {
	for i, w := range t.writers {
		if w == nil {
			continue
		}
		n, err = w.Write(buf)
		if err != nil {
			// ignore Writers that have errored. report last error only
			t.errs++
			t.writers[i] = nil
			if t.errs == len(t.writers) {
				return 0, err
			}
		}
	}
	return n, err
}

func main_parent() {
	host, err := os.Hostname()
	if err != nil {
		log.Println("os.Hostname failed:", err)
		host = "localhost"
	}
	cachename := fmt.Sprintf("%s.%d", host, os.Getpid())
	cmd := exec.Command(os.Args[0])
	cmd.Args = make([]string, len(os.Args) + 1)
	cmd.Args[0] = os.Args[0]
	cmd.Args[1] = fmt.Sprintf("-cache=%s", cachename)
	for i := 1; i < len(os.Args); i++ {
		cmd.Args[i+1] = os.Args[i]
	}
	logname := fmt.Sprintf("%s.%d.log", host, os.Getpid())
	logpath := filepath.Join(App.Cache, logname)
	logfile, err := fs.CreateIn(App.Cache, logname)
	var logger io.Writer
	if err != nil {
		log.Println("error creating log file: ", err)
		logger = os.Stderr
	} else {
		logger = &TeeLogger{writers: []io.Writer{os.Stderr, logfile}}
	}
	cmd.Stdout = logger
	cmd.Stderr = logger
	status := 0
	err = cmd.Start()
	if err != nil {
		fatal("launch error:", err)
	}
	sigs := make(chan os.Signal)
	signal.Notify(sigs)
	go func() {
		for sig := range sigs {
			// propagate signal to child process. we'll die when it dies.
			cmd.Process.Signal(sig)
		}
	}()
	state, err := cmd.Process.Wait()
	if err != nil {
		log.Println("wait error:", err)
	}
	if state != nil && !state.Success() {
		status = 1
	}
	os.Remove(filepath.Join(App.Cache, cachename))
	if status == 0 && logfile != nil {
		logfile.Close()
		os.Remove(logpath)
	}
	os.Exit(status)
}

func main_child() {
	var err error
	log.Println("sqribe version unknown")

	if *profile != "" {
		f, err := os.Create(*profile)
		if err != nil {
			fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	err = audio.Open()
	if err != nil {
		fatal(err)
	}

	G.plumb.selection = plumb.MkPort()
	G.plumb.score = plumb.MkPort()

	G.score = score.MkScore(G.plumb.score)

	G.font.luxi = mustMkFont(MustFind("luxisr.ttf"), 10)
	G.noteMenu = mkMenu(StringMenuOps{}, "1/16", "1/8", "1/4", "1/2", "1", "2", "3", "4")
	G.noteMenu.SetDefault("1")
	G.instMenu = mkMenu(StringMenuOps{toStr: func(item interface{})string {return midi.InstName(item.(int))}}, midi.InstPiano, midi.InstEPiano, midi.InstGuitar, midi.InstEGuitar, midi.InstMuteGuitar, midi.InstViolin, midi.InstHarp, midi.InstVoice)

	Synth, err = SynthInit(audio.SampleRate, MustFind("FluidR3_GM.sf2"))
	if err != nil {
		fatal(err)
	}

	redraw := make(chan Widget, 10)

	G.ww = NewWaveWidget(redraw)
	G.ww.SetScore(G.score)

	G.mixw = NewMixWidget(redraw)

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
		if err = open(audioFile); err != nil {
			alert("%v", err)
		} else if *initialTime != 0 && G.wav != nil {
			G.ww.ScrollToFrame(G.wav.FrameAtTime(*initialTime))
		}
	}

	redraw <- nil

	wg.Wait()

	audio.Shutdown()
}
