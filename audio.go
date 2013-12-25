package main

import (
	"code.google.com/p/portaudio-go/portaudio"
	"runtime"
	"sync"
)

type RingBuffer struct {
	buf []int16
	head int
	tail int
	write *sync.Cond
}

var buf *RingBuffer
var playing bool
var stream *portaudio.Stream

func AudioInit() (uint8, uint32, error) {
	err := portaudio.Initialize()
	if err != nil {
		return 0, 0, err
	}

	runtime.GOMAXPROCS(runtime.GOMAXPROCS(0) + 1)

	jack, err := portaudio.HostApi(portaudio.JACK)
	dev := jack.Devices[1]
	params := portaudio.HighLatencyParameters(nil, dev)
	s, err := portaudio.OpenStream(params, paCallback)
	if err != nil {
		return 0, 0, err
	}
	s16PerSecond := int(params.SampleRate) * params.Output.Channels
	buf = NewRingBuffer(s16PerSecond/2)
	stream = s
	return uint8(params.Output.Channels), uint32(params.SampleRate), nil
}

func AudioShutdown() {
	portaudio.Terminate()
}

func AppendAudio(src []int16) int {
	return buf.Append(src)
}

func NewRingBuffer(bufSize int) *RingBuffer {
	var ring RingBuffer
	ring.buf = make([]int16, bufSize)
	ring.write = sync.NewCond(&sync.Mutex{})
	return &ring
}

/* appends int16s. if the ring buffer is full, Append blocks until the samples can fit */
func (ring *RingBuffer) Append(src []int16) int {
	ring.write.L.Lock()
	defer ring.write.L.Unlock()
	for len(src) > len(ring.buf) - ring.Size() {
		ring.write.Wait()
		if !playing {
			return -1
		}
	}
	newTail := ring.tail + len(src)
	if newTail > len(ring.buf) {
		newTail %= len(ring.buf)
		n1 := copy(ring.buf[ring.tail:], src)
		copy(ring.buf[:newTail], src[n1:])
	} else {
		copy(ring.buf[ring.tail:newTail], src)
	}
	ring.tail = newTail
	return len(src)
}

/* tries to fill the dest buffer with int16s. if not enough s16s are in the ring buffer,
 * the remaining items in the output buffer are left untouched. */
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
	ring.write.Signal() // there might be a thread waiting for space in Append
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

func paCallback(out []int16) {
	if !playing {
		return
	}
	buf.Extract(out)
}

func StartPlayback() {
	playing = true
	stream.Start()
}

func StopPlayback() {
	go func() {
		playing = false
		stream.Abort()
		buf.write.Signal()
		buf.Clear()
	}()
}

func IsPlaying() bool {
	return playing
}
