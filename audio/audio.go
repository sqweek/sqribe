package audio

import (
	"code.google.com/p/portaudio-go/portaudio"
	"errors"
	"flag"
	"log"
	"runtime"

	. "sqweek.net/sqribe/core/types"
)

type audioOps interface {
	Open(params portaudio.StreamParameters) (*portaudio.Stream, error)
	Append(samples []int16) int
	Start()
	Index() (idx FrameN, ok bool)
}

var useCallback = flag.Bool("cb", false, "use callback")

var ops audioOps
var stream *portaudio.Stream

var stopped bool = true
var fr, prevfr FrameRange
var baseIndex, prevBase FrameN

func HostApi() *portaudio.HostApiInfo {
	/* TODO allow user to override host api */
	for _, api := range PlatformHostApis() {
		hostApi, err := portaudio.HostApi(api)
		if err == nil {
			return hostApi
		}
		log.Println(err)
	}
	return nil
}

var (
	Channels int
	SampleRate int // aka Frame rate
)

func Open() error {
	err := portaudio.Initialize()
	if err != nil {
		return err
	}

	runtime.GOMAXPROCS(runtime.GOMAXPROCS(0) + 1)

	host := HostApi()
	if host == nil {
		return errors.New("no host APIs available!")
	}
	dev := host.DefaultOutputDevice
	params := portaudio.LowLatencyParameters(nil, dev)
	params.SampleRate = 44100
	if *useCallback {
		ops = cbOps()
	} else {
		ops = blockOps(params.Output.Channels)
	}
	s, err := ops.Open(params)
	if err != nil {
		return err
	}
	stream = s
	Channels = params.Output.Channels
	SampleRate = int(params.SampleRate)

	impl := "blocking"
	if (*useCallback) {
		impl = "callback"
	}
	log.Printf("audio %s stream %s:'%s' (%d channels @ %d Hz)\n", impl, host.Name, dev.Name, Channels, SampleRate)

	return nil
}

func Shutdown() {
	portaudio.Terminate()
}

func Append(wav []int16) int {
	n := ops.Append(wav)
	fr.Max += FrameN(n / Channels)
	return n
}

func Play(f0 FrameN) {
	prevfr, prevBase = fr, baseIndex
	fr = FrameRange{f0, f0}
	baseIndex += (prevfr.Max - prevfr.Min)
	if stopped {
		stopped = false
		ops.Start()
		stream.Start()
	}
}

func Stop() {
	stopped = true
	stream.Abort()
}

func IsPlaying() bool {
	return !stopped
}

func PlayingFrame() (FrameN, bool) {
	if stopped {
		return 0, false
	}
	index, ok := ops.Index()
	if index < baseIndex {
		/* haven't looped around yet */
		return prevfr.Min + (index - prevBase), ok
	}
	return fr.Min + (index - baseIndex), ok
}
