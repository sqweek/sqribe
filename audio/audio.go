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
	Index() (idx SampleN, ok bool)
}

var useCallback = flag.Bool("cb", false, "use callback")

var ops audioOps
var stream *portaudio.Stream
var samplesPerSecond float64

var (
	currentS0 SampleN
	currentLen SampleN = 0
)

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
	s16PerSecond := int(params.SampleRate) * params.Output.Channels
	stream = s
	samplesPerSecond = float64(s16PerSecond)
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
	return ops.Append(wav)
}

func Play(s0, period SampleN) {
	if period == 0 {
		return
	}
	currentLen = period
	currentS0 = s0
	ops.Start()
	stream.Start()
}

func Stop() {
	currentLen = 0
	stream.Abort()
}

func IsPlaying() bool {
	return currentLen != 0
}

func PlayingSample() (SampleN, bool) {
	if currentLen == 0 {
		return 0, false
	}
	index, ok := ops.Index()
	return currentS0 + (index % currentLen), ok
}
