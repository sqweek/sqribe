package audio

import (
	"code.google.com/p/portaudio-go/portaudio"
	"errors"
	"flag"
	"time"

	"sqweek.net/sqribe/log"

	. "sqweek.net/sqribe/core/types"
)

type audioOps interface {
	Open(params portaudio.StreamParameters) (*portaudio.Stream, error)
	Append(samples []int16) int
	Prepare()
	Started()
	Index() (idx FrameN, ok bool)
}

var useCallback = flag.Bool("cb", true, "use callback")

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
		log.AU.Printf("%v: %v", api, err)
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

	host := HostApi()
	if host == nil {
		return errors.New("no host APIs available!")
	}
	dev := host.DefaultOutputDevice
	params := portaudio.LowLatencyParameters(nil, dev)
	l := params.Output.Latency
	/* pulseaudio (via ALSA) uses heaps of CPU at the default low latency (~8ms) */
	for params.Output.Latency < 30 * time.Millisecond {
		if params.Output.Latency + l > dev.DefaultHighOutputLatency {
			params.Output.Latency = dev.DefaultHighOutputLatency
			break
		}
		params.Output.Latency += l
	}
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
	log.AU.Printf("%s stream %s:'%s' (%d channels @ %d Hz) w/ latency %v\n", impl, host.Name, dev.Name, Channels, SampleRate, params.Output.Latency)

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

func Play(f0 FrameN) error {
	if stopped {
		baseIndex = 0
		ops.Prepare()
		if err := stream.Start(); err != nil {
			return err
		}
		ops.Started()
		stopped = false
	} else {
		prevfr, prevBase = fr, baseIndex
		baseIndex += (prevfr.Max - prevfr.Min)
	}
	fr = FrameRange{f0, f0}
	return nil
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
