package main

import (
	"time"

	"github.com/sqweek/fluidsynth"
)

type Synthesizer struct {
	fluid *fluidsynth.Synth
	chans map[uint8]uint8 // midi instrument -> channel allocations
	schedule chan ScheduledEvent
	tuning float64
	freq float64
}

var Synth *Synthesizer

func SynthInit(srate int, sfont string) (*Synthesizer, error) {
	settings := make(map[string]interface{})
	settings["audio.period-size"] = srate
	settings["audio.sample-format"] = "16bits"
	settings["synth.gain"] = 0.6
	synth := &Synthesizer{
		fluid: fluidsynth.NewSynth(settings),
		chans: make(map[uint8]uint8),
		schedule: make(chan ScheduledEvent),
	}
	/* TODO load sound font in background */
	synth.fluid.SFLoad(sfont, true)
	go synth.scheduler()
	return synth, nil
}

func (s *Synthesizer) WriteFrames(buf []int16) {
	s.fluid.WriteFrames_int16(buf)
}

/* returns the channel allocated for a particular instrument */
func (s *Synthesizer) Inst(inst uint8) uint8 {
	c, ok := s.chans[inst]
	if !ok {
		c = uint8(len(s.chans))
		s.chans[inst] = c
		s.fluid.ProgramChange(c, inst)
		if s.tuning != 0 {
			s.fluid.TuningChange(c, 0)
		}
	}
	return c
}

func (s *Synthesizer) Tuning() float64 {
	return s.tuning
}

func (s *Synthesizer) TuningFreq() float64 {
	return s.freq
}

func (s *Synthesizer) AdjustTuning(Δcents float64) float64 {
	return s.SetTuning(s.tuning + Δcents)
}

func (s *Synthesizer) SetTuning(newTuning float64) (freq float64) {
	s.tuning = newTuning
	tuning := fluidsynth.ShiftedTuning(newTuning)
	s.fluid.InstallTuning(0, "sqribe", tuning)
	for _, ch := range s.chans {
		s.fluid.TuningChange(ch, 0)
	}
	freq = tuning.Freq(69) // 69 is "midi A5" aka "scientific pitch notation A4"
	s.freq = freq
	return
}

type SynthEvent interface {
	Trigger(s *Synthesizer)
}

type NoteOff struct {
	channel, note uint8
}

func (ev NoteOff) Trigger(s *Synthesizer) {
	s.fluid.NoteOff(ev.channel, ev.note)
}

type ScheduledEvent struct {
	deadline time.Time
	event SynthEvent
	next *ScheduledEvent
}

func (s *Synthesizer) NoteOn(channel, note, velocity uint8) {
	s.fluid.NoteOn(channel, note, velocity)
}

func (s *Synthesizer) NoteOff(channel, note uint8) {
	s.fluid.NoteOff(channel, note)
}

func (s *Synthesizer) Note(channel, note, velocity uint8, duration time.Duration) {
	s.fluid.NoteOn(channel, note, velocity)
	deadline := time.Now().Add(duration)
	s.schedule <- ScheduledEvent{deadline, NoteOff{channel, note}, nil}
}

func (s *Synthesizer) scheduler() {
	var pending *ScheduledEvent = nil
	var timer *time.Timer = nil
	var wake <-chan time.Time = nil
	resched := func() {
		now := time.Now()
		d := time.Duration(0)
		if !now.Before(pending.deadline) {
			d = pending.deadline.Sub(now)
		}
		if timer == nil {
			timer = time.NewTimer(d)
			wake = timer.C
		} else {
			timer.Reset(d)
		}
	}
	for {
		select {
		case event := <-s.schedule:
//			if pending == nil {
//				pending = &event
//				resched()
//				continue
//			} else if event.deadline.Before(pending.deadline) {
//				event.next = pending
//				pending = &event
//				resched()
//				continue
//			}
			var node **ScheduledEvent
			for node = &pending; *node != nil && !event.deadline.Before((*node).deadline); node = &((*node).next) { }
			event.next = *node
			*node = &event
			if node == &pending {
				resched()
			}
		case now := <-wake:
			timer, wake = nil, nil
			for pending != nil && now.Before(pending.deadline) {
				pending.event.Trigger(s)
				pending = pending.next
			}
			if pending != nil {
				resched()
			}
		}
	}
}
