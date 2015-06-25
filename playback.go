package main

import (
	"fmt"
	"time"

	"sqweek.net/sqribe/audio"
	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/score"

	. "sqweek.net/sqribe/core/types"
)

type Samples struct {
	buf []int16
	frame FrameN
}

type BeatEv struct {
	Frame FrameN
	Next *BeatEv
}

type MidiEv struct {
	Pitch uint8
	Chan uint8
	Velocity uint8
	Start, End FrameN
	Next *MidiEv
}

func midilst(f0, fN, fcur FrameN) (*MidiEv, *MidiEv) {
	var evcur, evhead *MidiEv
	evtail := &evhead
	next := G.score.Iter(FrameRange{f0, fN})
	var sn score.StaffNote
	for next != nil {
		sn, next = next()
		start, _ := G.score.ToFrame(G.score.Beatf(sn.Note))
		end, _ := G.score.ToFrame(G.score.Beatf(sn.Note) + sn.Note.Durf())
		if sn.Staff.Muted {
			continue
		}
		if end <= f0 {
			continue
		} else if start >= fN {
			break
		}
		if start < f0 {
			start = f0
		}
		if end > fN {
			end = fN
		}

		midichan := Synth.Inst(uint8(sn.Staff.Voice()))
		*evtail = &MidiEv{sn.Note.Pitch, midichan, uint8(sn.Staff.Velocity()), start, end, nil}
		if start >= fcur && evcur == nil {
			evcur = *evtail
		}
		evtail = &((*evtail).Next)
	}
	return evhead, evcur
}

func beatlst(f0, fN, fcur FrameN) (*BeatEv, *BeatEv) {
	var bcur, bhead *BeatEv
	btail := &bhead
	for _, frame := range G.score.BeatFrames() {
		if frame < f0 {
			continue
		} else if frame > fN {
			break
		}
		*btail = &BeatEv{frame, nil}
		if frame > fcur && bcur == nil {
			bcur = *btail
		}
		btail = &((*btail).Next)
	}
	return bhead, bcur
}

// linear interpolation between 'from' -> zero -> 'to'
func crossfade(from, to []int16, steps FrameN) []int16 {
	nchan := FrameN(len(from))
	out := make([]int16, nchan*steps)
	for i := FrameN(0); i < steps; i++ {
		α := 1.0 - float64(i + 1)/float64(steps + 1)
		for j := FrameN(0); j < nchan; j++ {
			if α > 0.5 {
				out[nchan*i + j] = int16(float64(from[j]) * 2 * (α - 0.5))
			} else {
				out[nchan*i + j] = int16(float64(to[j]) * 2 * (0.5 - α))
			}
		}
	}
	return out
}

type PlayChange struct {
	beat, note bool
}

func (pc *PlayChange) Empty() bool {
	return !pc.beat && !pc.note
}

func (pc *PlayChange) Update(event interface{}) {
	switch event.(type) {
	case score.BeatChanged:
		pc.beat = true
	case score.StaffChanged:
		pc.note = true
	}
}

func coalesced(out chan PlayChange) chan interface{} {
	in := make(chan interface{})
	go coalesce(in, out)
	return in
}

func coalesce(in chan interface{}, out chan PlayChange) {
	defer close(out)
	changes := PlayChange{}
	for in != nil {
		for changes.Empty() {
			if ev, open := <-in; open {
				changes.Update(ev)
			} else {
				return
			}
		}
		for !changes.Empty() {
			select {
			case ev2, open := <-in:
				if open {
					changes.Update(ev2)
				} else {
					in = nil
				}
			case out <- changes:
				changes = PlayChange{}
			}
		}
	}
}

const (
	STOPPED = iota
	PLAYING
	STOPPING
)

/* globally mutable state... that's not thinking with channels :S */
var playState int = STOPPED

func playToggle() {
	switch playState {
	case PLAYING:
		fmt.Println("stopping playback")
		playState = STOPPING
		return
	case STOPPING:
		return /* in transition; do nothing */
	}

	playState = PLAYING
	rng := G.ww.GetSelectedTimeRange()
	f0, fN := rng.MinFrame(), rng.MaxFrame()

	if f0 == fN {
		fN = G.wav.ToFrame(G.wav.NSamples) - 1
		f0 = G.ww.FrameAtCursor()
	}
	fmt.Println("starting playback", f0, fN)

	/* short crossfade to loop smoothly */
	N := fN - f0 + 1
	// pad to nearest 64th frame, minimum 20 frames
	nfPad := 19 + (64 - (N + 19) % 64)

	evhead, _ := midilst(f0, fN, f0)
	bhead, _ := beatlst(f0, fN, f0)

	padN := N + nfPad
	/* wave sample prefetch thread */
	sampch := make(chan Samples, 10)
	go func() {
		bufsiz := FrameN(2048) // must be multiple of 64
		var s Samples
		i := FrameN(0)
		frame0 := G.wav.Frames(f0, f0)
		for playState == PLAYING {
			s.frame = f0 + i
			if i + bufsiz > N {
				wave := G.wav.Frames(s.frame, fN)
				s.buf = make([]int16, len(wave) + int(nfPad)*len(frame0))
				copy(s.buf, wave)
				copy(s.buf[len(wave):], crossfade(wave[len(wave) - len(frame0):], frame0, nfPad))
			} else {
				s.buf = G.wav.Frames(s.frame, s.frame + bufsiz - 1)
			}
			nf := G.wav.ToFrame(SampleN(len(s.buf)))
			sampch <- s
			i += nf
			if i >= padN {
				i = 0
			}
		}
		close(sampch)
	}()
	scorechan := make(chan PlayChange)
	G.plumb.score.Sub(&playState, coalesced(scorechan))

	audio.Play(f0)
	/* synth & sample feeding thread */
	go func() {
		var in Samples
		woodblock := Synth.Inst(midi.InstWoodblock)
		bev := bhead
		bon := false
		nf := FrameN(64)
		bufsiz := int(G.wav.ToSample(nf))
		mbuf := make([]int16, bufsiz)
		mev := evhead
		offlist := make([]MidiEv, 0, 32)
		for playState == PLAYING {
			if len(in.buf) == 0 {
				prevframe := in.frame
				in = <-sampch
				if len(in.buf) < bufsiz || len(in.buf) % bufsiz != 0 {
					fmt.Println("prefetch samples sent in non-64 frame multiple", len(in.buf))
					playState = STOPPING
					break
				}
				select {
				case changed := <-scorechan:
					if changed.beat {
						bhead, bev = beatlst(f0, fN, in.frame)
					}
					if changed.note || changed.beat {
						evhead, mev = midilst(f0, fN, in.frame)
					}
				default:
				}
				if prevframe > in.frame {
					/* we just looped back around */
					mev = evhead
					bev = bhead
					audio.Play(in.frame)
				}
			}
			buf := in.buf[:bufsiz]
			in.buf = in.buf[bufsiz:]

			/* turn notes off first so notes at the same pitch directly following
			** one another don't get truncated */
			for j := len(offlist) - 1; j >= 0; j-- {
				// XXX sorted list might be simpler?
				if offlist[j].End < in.frame + nf {
					Synth.NoteOff(offlist[j].Chan, offlist[j].Pitch)
					if j == len(offlist) - 1 {
						offlist = offlist[:j]
					} else {
						copy(offlist[j:], offlist[j+1:])
					}
				}
			}
			/* metronome */
			if bon {
				Synth.NoteOff(woodblock, midi.PitchF6)
				bon = false
			} else if !Mixer.MuteMetronome {
				for bev != nil && bev.Frame < in.frame + nf {
					Synth.NoteOn(woodblock, midi.PitchF6, 120)
					bon = true
					bev = bev.Next
				}
			}
			/* user placed notes */
			for mev != nil && mev.Start < in.frame + nf {
				Synth.NoteOn(mev.Chan, mev.Pitch, mev.Velocity)
				offlist = append(offlist, *mev)
				mev = mev.Next
			}

			Synth.WriteFrames(mbuf)
			α, β := 0.0, 0.0
			bias := Mixer.Bias.Value()
			if !Mixer.MuteWave {
				α = 0.5 + bias
			}
			if !Mixer.MuteMidi {
				β = 0.5 - bias
			}
			for j := 0; j < bufsiz; j++ {
				mbuf[j] = int16(α * float64(buf[j]) + β * float64(mbuf[j]))
			}
			audio.Append(mbuf)
		}
		for _ = range(sampch) {
			// drain channel
		}
		G.plumb.score.Unsub(&playState)
		fmt.Println("notifying portaudio")
		audio.Stop()
		playState = STOPPED
		fmt.Println("playback all stopped")
	}()
	//TODO wait for ring buffer to fill up a bit before kicking off audio
	/* gui feedback thread */
	go func() {
		for {
			f, playing := audio.PlayingFrame()
			if !playing {
				if playState == PLAYING && f != 0 {
					/* we think we're playing, but the audio callback hasn't
					 * run for awhile. just stop. */
					fmt.Println("lost audio callback, stopping")
					playState = STOPPING
				}
				break
			}
			G.ww.SetCursorByFrame(f)
			time.Sleep(66 * time.Millisecond)
		}
	}()
}



