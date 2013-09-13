package main

import (
	"github.com/neagix/Go-SDL/sdl"
	"github.com/neagix/Go-SDL/sdl/audio"
	"unsafe"
	"reflect"
	"sync"
)

type RingBuffer struct {
	buf []int16
	head int
	tail int
	write *sync.Cond
}

var buf *RingBuffer

func AudioInit(desired *audio.AudioSpec) (*audio.AudioSpec, error) {
	var obtained audio.AudioSpec
	desired.UserDefinedCallback = callback
	if audio.OpenAudio(desired, &obtained) != 0 {
		return nil, &Errstr{sdl.GetError()}
	}
	if obtained.Format != audio.AUDIO_S16SYS {
		return &obtained, &Errstr{"only S16 supported"}
	}
	s16PerSecond := obtained.Freq * int(obtained.Channels)
	buf = NewRingBuffer(s16PerSecond/2)
	return &obtained, nil
}

func AppendAudio(src []int16) {
	buf.Append(src)
}

func NewRingBuffer(bufSize int) *RingBuffer {
	var ring RingBuffer
	ring.buf = make([]int16, bufSize)
	ring.write = sync.NewCond(&sync.Mutex{})
	return &ring
}

/* appends int16s. if the ring buffer is full, Append blocks until the samples can fit */
func (ring *RingBuffer) Append(src []int16) {
	ring.write.L.Lock()
	for len(src) > len(ring.buf) - ring.Size() {
		ring.write.Wait()
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
	ring.write.L.Unlock()
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
	if t < h {
		return len(ring.buf) + t - h
	}
	return t - h
}

func callback(outptr unsafe.Pointer, nbytes int) {
	var out []int16
	n := nbytes / 2
	hdr := (*reflect.SliceHeader)((unsafe.Pointer(&out)))
	hdr.Cap = n
	hdr.Len = n
	hdr.Data = uintptr(outptr)

	/* TODO if playback stopped, clear the ring buffer */

	buf.Extract(out)
}
