package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"strings"
	"fmt"
	"os"

	"sqweek.net/sqribe/fs"
	"sqweek.net/sqribe/score"

	. "sqweek.net/sqribe/core/types"
)

type state interface {
	Capture() // captures current memory model state
	Restore() // restores this objects state to the memory model
}

type stateV1 struct {
	Filename string
	Beats []FrameN
	Staves []score.SavedStaff
	MixWeight float64
	MetronomeOff bool `json:",omitempty"`
	WaveOff bool `json:",omitempty"`
	MidiOff bool `json:",omitempty"`
}

func (s *stateV1) Capture() {
	s.Filename = G.audiofile
	s.Beats = G.score.BeatFrames()
	s.Staves = G.score.SavedStaves(s.Beats)
	s.MixWeight = Mixer.Bias.Value()
	s.MetronomeOff = Mixer.MuteMetronome
	s.WaveOff = Mixer.MuteWave
	s.MidiOff = Mixer.MuteMidi
}

func (s *stateV1) Restore() {
	G.score.LoadBeats(s.Beats)
	G.score.LoadStaves(s.Staves, s.Beats)
	Mixer.Bias.Set(s.MixWeight)
	Mixer.MuteMetronome = s.MetronomeOff
	Mixer.MuteWave = s.WaveOff
	Mixer.MuteMidi = s.MidiOff
}

type VersionHeader struct {
	Version int
}

var currentVersion = VersionHeader{1}

func stateV(hdr VersionHeader) state {
	switch (hdr.Version) {
	case 1:
		return &stateV1{}
	}
	panic(fmt.Sprintf("unknown version %d", hdr.Version))
}

func flatpath(r rune) rune {
	if r < 26 || strings.ContainsRune(" /:\\", r) {
		return '_'
	}
	return r
}

func key(filename string) string {
	return strings.TrimLeft(strings.Map(flatpath, filename) + ".sqs", "_")
}

func LoadState(filename string) error {
	stateFile := fs.SaveDir() + "/" + key(filename)
	if _, err := os.Stat(stateFile); err == nil {
		f, err := os.Open(stateFile)
		if err != nil {
			return err
		}
		defer f.Close()
		j := json.NewDecoder(f)
		var version VersionHeader
		err = j.Decode(&version)
		if err != nil {
			return err
		}
		s := stateV(version)
		err = j.Decode(&s)
		if err != nil {
			return err
		}
		s.Restore()
	}
	return nil
}

func SaveState(filename string) error {
	k := key(filename)
	tmpfile, err := ioutil.TempFile(fs.SaveDir(), k)
	if err != nil {
		return err
	}
	err = WriteState(tmpfile)
	if err != nil {
		return err
	}
	err = fs.ReplaceFile(tmpfile.Name(), fs.SaveDir() + "/" + k)
	if err != nil {
		return err
	}
	return nil
}

func WriteState(tmpfile io.WriteCloser) error {
	defer tmpfile.Close()
	j := json.NewEncoder(tmpfile)
	j.Encode(&currentVersion)
	s := stateV(currentVersion)
	s.Capture()
	err := j.Encode(s)
	if err != nil {
		return err
	}
	return nil
}