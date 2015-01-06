package audio

import (
	"code.google.com/p/portaudio-go/portaudio"
	"time"

	. "sqweek.net/sqribe/core/types"
)

type callbackOps struct {
	buf *RingBuffer
	index SampleN
	timing portaudio.StreamCallbackTimeInfo
}



func cbOps() *callbackOps {
	/* TODO should be based on the actual buffer size */
	return &callbackOps{buf: NewRingBuffer(2048 * 3)}
}

func (cb *callbackOps) Open(params portaudio.StreamParameters) (*portaudio.Stream, error) {
	return portaudio.OpenStream(params, paCallback)
}

func (cb *callbackOps) Append(wav []int16) int {
	return cb.buf.Append(wav)
}

func paCallback(out []int16, time portaudio.StreamCallbackTimeInfo) {
	cb := ops.(*callbackOps)
	if currentLen == 0 {
		return
	}
	n := cb.buf.Extract(out)
	cb.index += SampleN(n)
	cb.index %= currentLen
	cb.timing = time
}

func (cb *callbackOps) Start() {
	cb.buf.Clear()
	cb.index = 0
}

func (cb *callbackOps) Index() (SampleN, bool) {
	if cb.timing.CurrentTime == 0 {
		return cb.index, true
	}
	secs := (stream.Time() - cb.timing.OutputBufferDacTime).Seconds()
	return cb.index + SampleN(samplesPerSecond * secs), secs < 0.5
}

type RingBuffer struct {
	buf []int16
	head int
	tail int
}

func NewRingBuffer(bufSize int) *RingBuffer {
	var ring RingBuffer
	ring.buf = make([]int16, bufSize + 1)
	return &ring
}

/* appends int16s. if the ring buffer is full, Append blocks until the samples can fit */
func (ring *RingBuffer) Append(wav []int16) int {
	for len(wav) >= len(ring.buf) - ring.Size() {
		time.Sleep(50 * time.Millisecond)
	}
	newTail := ring.tail + len(wav)
	if newTail > len(ring.buf) {
		newTail %= len(ring.buf)
		nw := copy(ring.buf[ring.tail:], wav)
		copy(ring.buf[:newTail], wav[nw:])
	} else {
		copy(ring.buf[ring.tail:newTail], wav)
	}
	ring.tail = newTail
	return len(wav)
}

/* tries to fill the dest buffer. if the ring buffer contains insufficient
 * samples, the remaining items in the output buffer are left untouched. */
func (ring *RingBuffer) Extract(dest []int16) int {
	n := ring.Size()
	if n == 0 {
		return 0
	} else if n > len(dest) {
		n = len(dest)
	}
	newHead := ring.head + n
	if newHead > len(ring.buf) {
		newHead %= len(ring.buf)
		n1 := copy(dest, ring.buf[ring.head:])
		copy(dest[n1:], ring.buf[:newHead])
	} else {
		copy(dest, ring.buf[ring.head:newHead])
	}
	ring.head = newHead
	return n
}

func (ring *RingBuffer) Clear() {
	ring.head = 0
	ring.tail = 0
}

func (ring *RingBuffer) Size() int {
	h := ring.head
	t := ring.tail
	var s int
	if t < h {
		s = len(ring.buf) + t - h
	} else {
		s = t - h
	}
	return s
}

