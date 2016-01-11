package main

import (
	"math"
	"time"

	"sqweek.net/sqribe/audio"
	"sqweek.net/sqribe/log"
	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/score"

	. "sqweek.net/sqribe/core/types"
)

type Samples struct {
	buf []int16
	frame, f0, fN FrameN
}

type BeatEv struct {
	Frame FrameN
	Next *BeatEv
}

type MidiOff struct {
	End FrameN
	Pitch uint8
	Chan uint8
}

type MidiEv struct {
	Start FrameN
	Mix *StaffMix
	Off MidiOff
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
		end, _ := G.score.ToFrame(G.score.EndBeatf(sn.Note))
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

		mix := Mixer.For(sn.Staff)
		*evtail = &MidiEv{start, mix, MidiOff{end, sn.Note.Pitch, 255}, nil}
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
		log.AU.Println("stopping playback")
		playState = STOPPING
		return
	case STOPPING:
		return /* in transition; do nothing */
	}

	if G.wav == nil {
		return
	}
	playState = PLAYING
	rng := G.ww.SelectedTimeRange()
	if rng.MinFrame() == rng.MaxFrame() {
		play(FrameRange{G.ww.FrameAtCursor(), G.wav.ToFrame(G.wav.NSamples) - 1})
	} else {
		play(rng)
	}
}

func play(rng TimeRange) {
	if rng.MinFrame() < 0 {
		rng = FrameRange{0, rng.MaxFrame()}
	}
	log.AU.Println("starting loop", rng.MinFrame(), rng.MaxFrame())

	/* wave sample prefetch thread */
	sampch := make(chan Samples, 25)
	go func() {
		bufsiz := FrameN(2048) // must be multiple of 64
		var s Samples
		s.frame = rng.MinFrame()
		for playState == PLAYING {
			/* re-evaluate f0/fN each iteration in case a bounding beat moves */
			s.f0, s.fN = rng.MinFrame(), rng.MaxFrame()
			if s.frame + bufsiz > s.fN {
				// pad to nearest 64th frame, minimum 20 frames
				nfPad := 19 + (64 - ((s.fN - s.frame + 1) + 19) % 64)
				wave := G.wav.Frames(s.frame, s.fN)
				frame0 := G.wav.Frames(s.f0, s.f0)
				s.buf = make([]int16, len(wave) + int(nfPad)*len(frame0))
				copy(s.buf, wave)
				copy(s.buf[len(wave):], crossfade(wave[len(wave) - len(frame0):], frame0, nfPad))
			} else {
				s.buf = G.wav.Frames(s.frame, s.frame + bufsiz - 1)
			}
			nf := G.wav.ToFrame(SampleN(len(s.buf)))
			sampch <- s
			s.frame += nf
			if s.frame >= s.fN {
				s.frame = s.f0
			}
		}
		close(sampch)
	}()
	if err := audio.Play(rng.MinFrame()); err != nil {
		log.AU.Println("couldn't start stream:", err)
		playState = STOPPED
		return
	}
	scorechan := make(chan PlayChange)
	G.plumb.score.Sub(&playState, coalesced(scorechan))

	var mpeak, wpeak float64 = 0, 0
	/* synth & sample feeding thread */
	go func() {
		var in Samples
		var cutoff FrameN
		woodblock := Synth.Inst(midi.InstWoodblock)
		bhead, bev := beatlst(rng.MinFrame(), rng.MaxFrame(), rng.MinFrame())
		bon := false
		nf := FrameN(64)
		bufsiz := int(G.wav.ToSample(nf))
		mbuf := make([]int16, bufsiz)
		evhead, mev := midilst(rng.MinFrame(), rng.MaxFrame(), rng.MinFrame())
		offlist := make([]MidiOff, 0, 32)
		for playState == PLAYING {
			if len(in.buf) == 0 {
				prevframe := in.frame
				in = <-sampch
				if len(in.buf) < bufsiz || len(in.buf) % bufsiz != 0 {
					log.AU.Println("stopping: prefetch samples sent in non-64 frame multiple", len(in.buf))
					playState = STOPPING
					break
				}
				select {
				case changed := <-scorechan:
					start := time.Now()
					if changed.beat {
						bhead, bev = beatlst(in.f0, in.fN, in.frame)
					}
					if changed.note || changed.beat {
						evhead, mev = midilst(in.f0, in.fN, in.frame)
					}
					log.AU.Printf("playback change processed in %v (beats:%t notes:%t)", time.Now().Sub(start), changed.beat, changed.note)
				default:
				}
				if prevframe > in.frame {
					/* we just looped back around */
					mev = evhead
					bev = bhead
					audio.Play(in.frame)
				}
				cutoff = in.frame
			}
			buf := in.buf[:bufsiz]
			in.buf = in.buf[bufsiz:]
			cutoff += nf

			/* turn notes off first so notes at the same pitch directly following
			** one another don't get truncated */
			for j := len(offlist) - 1; j >= 0; j-- {
				// XXX sorted list might be simpler?
				if offlist[j].End < cutoff {
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
				for bev != nil && bev.Frame < cutoff {
					if !bon {
						Synth.NoteOn(woodblock, midi.PitchF6, 120)
						bon = true
					}
					bev = bev.Next
				}
			}
			/* user placed notes */
			for mev != nil && mev.Start < cutoff {
				if !mev.Mix.Muted {
					mev.Off.Chan = Synth.Inst(uint8(mev.Mix.Voice))
					Synth.NoteOn(mev.Off.Chan, mev.Off.Pitch, uint8(mev.Mix.Velocity))
					offlist = append(offlist, mev.Off)
				}
				mev = mev.Next
			}

			Synth.WriteFrames(mbuf)
			α, β := 0.0, 0.0
			if !Mixer.Wave.Muted {
				α = Mixer.Wave.Gain
			}
			if !Mixer.Midi.Muted {
				β = Mixer.Midi.Gain
			}
			γ := Mixer.Master.Gain
			agc := 1.0
			for j := 0; j < bufsiz; j++ {
				w, m := γ * α * float64(buf[j]), γ * β * float64(mbuf[j])
				wpeak = math.Max(wpeak, math.Abs(w))
				mpeak = math.Max(mpeak, math.Abs(m))
				if math.Abs(agc*(w + m)) > 32700 {
					f := math.Abs(w + m) / 32700
					for k := j - 1; k >= 0; k-- {
						mbuf[k] = int16(float64(mbuf[k]) / f)
					}
					agc /= f
				}
				mbuf[j] = int16(agc*(w + m))
			}
			if agc != 1.0 {
				Mixer.Master.Gain = γ * agc
			}
			audio.Append(mbuf)
		}
		for _, ev := range(offlist) {
			Synth.NoteOff(ev.Chan, ev.Pitch)
		}
		for _ = range(sampch) {
			// drain channel
		}
		G.plumb.score.Unsub(&playState)
		audio.Stop()
		playState = STOPPED
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
					log.AU.Println("stopping: lost audio callback")
					playState = STOPPING
				}
				break
			}
			G.ww.SetCursorByFrame(f)
			m, w := mpeak, wpeak
			mpeak, wpeak = 0, 0
			G.mixw.Levels(m/32700, w/32700)
			time.Sleep(66 * time.Millisecond)
		}
	}()
}



