package main

import (
	"encoding/json"
	"math/big"
	"io"
	"io/ioutil"
	"strings"
	"fmt"
	"os"

	"sqweek.net/sqribe/fs"
	"sqweek.net/sqribe/score"

	. "sqweek.net/sqribe/core/types"
)

type SavedStaff struct {
	Name string
	Voice int
	Velocity int
	Origin uint8
	Nsharps int
	Muted bool `json:",omitempty"`
	Notes []SavedNote
}

type SavedNote struct {
	Pitch uint8
	Duration *big.Rat
	Offset *big.Rat
}

type state interface {
	Capture() // captures current memory model state
	Restore() // restores this objects state to the memory model
}

type stateV1 struct {
	Filename string
	Beats []FrameN
	Staves []SavedStaff
	MixWeight float64
	MetronomeOff bool `json:",omitempty"`
	WaveOff bool `json:",omitempty"`
	MidiOff bool `json:",omitempty"`
}

func savedNotes(staff *score.Staff, beats []FrameN) []SavedNote {
	notes := staff.Notes()
	saved := make([]SavedNote, 0, len(notes))
	i := 0
	for _, note := range notes {
		for beats[i] < note.Beat.Frame() {
			i++
		}
		b := big.NewRat(int64(i), 1)
		b.Add(b, note.Offset)
		saved = append(saved, SavedNote{note.Pitch, note.Duration, b})
	}
	return saved
}

func loadNotes(sc *score.Score, staff *score.Staff, saved []SavedNote, beats []FrameN) []*score.Note {
	notes := make([]*score.Note, 0, len(saved))
	beat := sc.Head
	for _, sv := range saved {
		beatf, _ := sv.Offset.Float64()
		i := int(beatf)
		for beat.Frame() < beats[i] {
			beat = beat.Next()
		}
		sv.Offset.Sub(sv.Offset, big.NewRat(int64(i), 1))
		notes = append(notes, &score.Note{sv.Pitch, sv.Duration, beat, sv.Offset})
	}
	return notes
}

func savedStaves(score *score.Score, beats []FrameN) []SavedStaff {
	staves := score.Staves()
	saved := make([]SavedStaff, 0, len(staves))
	for _, staff := range staves {
		notes := savedNotes(staff, beats)
		mix := Mixer.For(staff)
		saved = append(saved, SavedStaff{staff.Name(), mix.Voice, mix.Velocity - 100, staff.Clef().Origin, int(staff.Key()), mix.Muted, notes})
	}
	return saved
}

func loadStaves(sc *score.Score, saved []SavedStaff, beats []FrameN)  {
	staves := make([]*score.Staff, 0, len(saved))
	for _, sv := range saved {
		clef := score.FindClef(sv.Origin)
		if clef == nil {
			clef = &score.TrebleClef
		}
		staff := score.MkStaff(sv.Name, clef, score.KeySig(sv.Nsharps))
		sc.AddNotes(staff, loadNotes(sc, staff, sv.Notes, beats)...)
		staves = append(staves, staff)
		fmt.Println(staff)
		Mixer.LoadStaff(staff, sv)
	}
	sc.SetStaves(staves)
}

func (s *stateV1) Capture() {
	s.Filename = G.audiofile
	s.Beats = G.score.BeatFrames()
	s.Staves = savedStaves(&G.score, s.Beats)
	s.MixWeight = Mixer.Bias.Value()
	s.MetronomeOff = Mixer.MuteMetronome
	s.WaveOff = Mixer.MuteWave
	s.MidiOff = Mixer.MuteMidi
}

func (s *stateV1) Restore() {
	G.score.LoadBeats(s.Beats)
	loadStaves(&G.score, s.Staves, s.Beats)
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