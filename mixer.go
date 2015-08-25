package main

import (
	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/score"

	. "sqweek.net/sqribe/core/data"
)

type MixConfig struct {
	Bias *BoundFloat
	MuteWave, MuteMidi, MuteMetronome bool
	Staff map[*score.Staff]*StaffMix
}

type StaffMix struct {
	Voice int
	Velocity int
	Muted bool
}

var Mixer MixConfig

func init() {
	Mixer.Staff = make(map[*score.Staff]*StaffMix)
}

func (m *MixConfig) LoadStaff(staff *score.Staff, saved SavedStaff) {
	stm := m.For(staff)
	stm.Voice = saved.Voice
	stm.Velocity = saved.Velocity + 100
	stm.Muted = saved.Muted
}

func (m *MixConfig) For(staff *score.Staff) *StaffMix {
	if sm, ok := m.Staff[staff]; ok {
		return sm
	}
	m.Staff[staff] = &StaffMix{midi.InstPiano, 100, false}
	return m.Staff[staff]
}
