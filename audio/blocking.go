package audio

import (
	"code.google.com/p/portaudio-go/portaudio"
	"time"

	. "sqweek.net/sqribe/core/types"
)

type blockingOps struct {
	buf []int16
	playbackStart time.Duration
}

func blockOps(channels int) *blockingOps {
	return &blockingOps{buf: make([]int16, 64 * channels)}
}

func (block *blockingOps) Open(params portaudio.StreamParameters) (*portaudio.Stream, error) {
	return portaudio.OpenStream(params, block.buf)
}

func (block *blockingOps) Append(wav []int16) int {
	if len(wav) != len(block.buf) {
		panic(len(wav))
	}
	copy(block.buf, wav)
	stream.Write()
	return len(wav)
}

func (block *blockingOps) Start() {
	block.playbackStart = monotonicTime()
}

func (block *blockingOps) Index() (SampleN, bool) {
	dt := monotonicTime() - block.playbackStart
	return SampleN(samplesPerSecond * dt.Seconds()), true
}

