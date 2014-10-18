package main

import (
	"code.google.com/p/portaudio-go/portaudio"
	"log"
	"runtime"
	"sync"
)

type RingBuffer struct {
	wbuf []int16 //waveform
	mbuf []int16 //midi
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

func AudioInit() (uint8, uint32, error) {
	err := portaudio.Initialize()
	if err != nil {
		return 0, 0, err
	}

	runtime.GOMAXPROCS(runtime.GOMAXPROCS(0) + 1)

	host := HostApi()
	if host == nil {
		log.Fatal("no host APIs available!")
	}
	dev := host.DefaultOutputDevice
	params := portaudio.HighLatencyParameters(nil, dev)
	s, err := portaudio.OpenStream(params, paCallback)
	if err != nil {
		return 0, 0, err
	}
	s16PerSecond := int(params.SampleRate) * params.Output.Channels
	buf = NewRingBuffer(s16PerSecond/2)
	stream = s
	samplesPerSecond = float64(s16PerSecond)
	return uint8(params.Output.Channels), uint32(params.SampleRate), nil
}

func AudioShutdown() {
	portaudio.Terminate()
}

func AppendAudio(wav, midi []int16) int {
	return buf.Append(wav, midi)
}

func NewRingBuffer(bufSize int) *RingBuffer {
	var ring RingBuffer
	ring.wbuf = make([]int16, bufSize)
	ring.mbuf = make([]int16, bufSize)
	ring.write = sync.NewCond(&sync.Mutex{})
	return &ring
}

/* appends int16s. if the ring buffer is full, Append blocks until the samples can fit */
func (ring *RingBuffer) Append(wav, midi []int16) int {
	ring.write.L.Lock()
	defer ring.write.L.Unlock()
	for len(wav) > len(ring.wbuf) - ring.Size() {
		ring.write.Wait()
		if currentLen == 0 {
			return -1
		}
	}
	newTail := ring.tail + len(wav)
	if newTail > len(ring.wbuf) {
		newTail %= len(ring.wbuf)
		nm := copy(ring.mbuf[ring.tail:], midi)
		copy(ring.mbuf[:newTail], midi[nm:])
		nw := copy(ring.wbuf[ring.tail:], wav)
		copy(ring.wbuf[:newTail], wav[nw:])
	} else {
		copy(ring.mbuf[ring.tail:newTail], midi)
		copy(ring.wbuf[ring.tail:newTail], wav)
	}
	ring.tail = newTail
	return len(wav)
}

/* tries to fill the dest buffer by mixing the waveform & midi samples.
 * if the ring buffer contains insufficient samples, the remaining items
 * in the output buffer are left untouched. */
func (ring *RingBuffer) Mix(dest []int16, volw, volm float64) int {
	n := ring.Size()
	if n == 0 {
		return 0
	} else if n > len(dest) {
		n = len(dest)
	}
	newHead := ring.head + n
	if newHead > len(ring.wbuf) {
		newHead %= len(ring.wbuf)
		nm := ring.mix(dest, ring.head, len(ring.wbuf), volw, volm)
		ring.mix(dest[nm:], 0, newHead, volw, volm)
	} else {
		ring.mix(dest, ring.head, newHead, volw, volm)
	}
	ring.head = newHead
	// XXX we are in the audio callback; is Signal guaranteed not to block?
	ring.write.Signal() // there might be a thread waiting for space in Append
	return n
}

func (ring *RingBuffer) mix(dest []int16, i0, iN int, volw, volm float64) int {
	n := iN - i0
	for i := 0; i < n; i++ {
		ir := i0 + i
		dest[i] = int16(float64(ring.wbuf[ir]) * volw + float64(ring.mbuf[ir]) * volm)
	}
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
		s = len(ring.wbuf) + t - h
	} else {
		s = t - h
	}
	return s
}

func mixVolumes() (audio, midi float64) {
	audio, midi = 0, 0
	bias := G.mixer.waveBias
	if G.mixer.audio {
		audio = bias
	}
	if G.mixer.midi {
		midi = 1 - bias
	}
	return audio, midi
}

func paCallback(out []int16, time portaudio.StreamCallbackTimeInfo) {
	if currentLen == 0 {
		return
	}
	w, v := mixVolumes()
	n := buf.Mix(out, w, v)
	currentIndex += SampleN(n)
	currentIndex %= currentLen
	currentTime = time
}

func StartPlayback(s0, period SampleN) {
	if period == 0 {
		return
	}
	currentIndex = 0
	currentLen = period
	currentTime.OutputBufferDacTime = stream.Time()
	currentS0 = s0
	stream.Start()
}

func StopPlayback() {
	go func() {
		currentIndex = 0
		currentLen = 0
		stream.Abort()
		buf.write.Signal()
		buf.Clear()
	}()
}

func IsPlaying() bool {
	return currentLen != 0
}

func CurrentSample() (SampleN, bool) {
	if currentLen == 0 {
		return 0, false
	}
	dt := stream.Time() - currentTime.OutputBufferDacTime
	index := currentIndex + SampleN(samplesPerSecond * dt.Seconds())
	if index < 0 {
		return currentS0, true
	}
	index %= currentLen
	return currentS0 + index, true
}
