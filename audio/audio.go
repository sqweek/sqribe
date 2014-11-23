package audio

import (
	"code.google.com/p/portaudio-go/portaudio"
	"errors"
	"log"
	"runtime"
	"sync"

	. "sqweek.net/sqribe/core/types"
)

type RingBuffer struct {
	buf []int16
	head int
	tail int
	write *sync.Cond
}

var buf *RingBuffer
var stream *portaudio.Stream
var samplesPerSecond float64

var (
	currentS0 SampleN
	currentLen SampleN = 0
	currentIndex SampleN
	currentTime portaudio.StreamCallbackTimeInfo
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
	Channels uint8
	SampleRate uint32
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
	s, err := portaudio.OpenStream(params, paCallback)
	if err != nil {
		return err
	}
	s16PerSecond := int(params.SampleRate) * params.Output.Channels
	/* TODO should be based on the actual buffer size */
	buf = NewRingBuffer(2048 * 3)
	stream = s
	samplesPerSecond = float64(s16PerSecond)
	Channels = uint8(params.Output.Channels)
	SampleRate = uint32(params.SampleRate)
	return nil
}

func Shutdown() {
	portaudio.Terminate()
}

func Append(wav []int16) int {
	return buf.Append(wav)
}

func Clear() {
	buf.Clear()
}

func NewRingBuffer(bufSize int) *RingBuffer {
	var ring RingBuffer
	ring.buf = make([]int16, bufSize + 1)
	ring.write = sync.NewCond(&sync.Mutex{})
	return &ring
}

/* appends int16s. if the ring buffer is full, Append blocks until the samples can fit */
func (ring *RingBuffer) Append(wav []int16) int {
	ring.write.L.Lock()
	defer ring.write.L.Unlock()
	for len(wav) >= len(ring.buf) - ring.Size() {
		ring.write.Wait()
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
	/* cond.Signal() can technically block, as it acquires the Lock. However it
	  only does so if there is a something waiting on the condition, and since
	  we only ever have a single thread that might be waiting we should be
	  able to acquire the lock uncontested */
	ring.write.Signal() // there might be a thread waiting for space in Append
	return n
}

func (ring *RingBuffer) Clear() {
	ring.head = 0
	ring.tail = 0
	ring.write.Signal()
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

func paCallback(out []int16, time portaudio.StreamCallbackTimeInfo) {
	if currentLen == 0 {
		return
	}
	n := buf.Extract(out)
	currentIndex += SampleN(n)
	currentIndex %= currentLen
	currentTime = time
}

func Play(s0, period SampleN) {
	if period == 0 {
		return
	}
	currentIndex = 0
	currentLen = period
	currentTime.OutputBufferDacTime = stream.Time()
	currentS0 = s0
	stream.Start()
}

func Stop() {
	go func() {
		currentIndex = 0
		currentLen = 0
		stream.Abort()
	}()
}

func IsPlaying() bool {
	return currentLen != 0
}

func PlayingSample() (SampleN, bool) {
	if currentLen == 0 {
		return 0, false
	}
	dt := stream.Time() - currentTime.OutputBufferDacTime
	index := currentIndex + SampleN(samplesPerSecond * dt.Seconds())
	/* if audio callback hasn't run for half a second we're in trouble */
	ok := dt.Seconds() < 0.5
	if index < 0 {
		return currentS0, ok
	}
	return currentS0 + (index % currentLen), ok
}
