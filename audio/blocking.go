package audio

import (
	"github.com/gordonklaus/portaudio"

	"github.com/sqweek/sqribe/log"
	. "github.com/sqweek/sqribe/core/types"
)

type blockingOps struct {
	buf []int16
	pos int
	writes int
	frameDelay FrameN
}

func blockOps(channels int) *blockingOps {
	return &blockingOps{buf: make([]int16, 1024 * channels)}
}

func (block *blockingOps) Open(params portaudio.StreamParameters) (s *portaudio.Stream, err error) {
	s, err = portaudio.OpenStream(params, block.buf)
	if err == nil {
		info := s.Info()
		block.frameDelay = FrameN(info.OutputLatency.Seconds() * float64(info.SampleRate))
		log.AU.Println("frameDelay", block.frameDelay)
	}
	return
}

func (block *blockingOps) Append(wav []int16) int {
	src := wav
	for len(src) > 0 {
		n := copy(block.buf[block.pos:], src)
		src = src[n:]
		block.pos += n
		if block.pos == len(block.buf) {
			stream.Write()
			block.pos = 0
			block.writes++
		}
	}
	return len(wav)
}

func (block *blockingOps) Prepare() {
	block.pos = 0
	block.writes = 0
}

func (block *blockingOps) Started() {
}

func (block *blockingOps) Index() (FrameN, bool) {
	written := FrameN((block.writes * len(block.buf) + block.pos) / Channels)
	if written < block.frameDelay {
		return 0, true
	}
	return written - block.frameDelay, true
}
