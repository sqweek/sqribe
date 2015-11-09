package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/big"
	"strings"
	"os"

	"sqweek.net/sqribe/audio"
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
	Capture(h *Headers) // captures current memory model state
	Restore() // restores this objects state to the memory model
}

type stateV2 struct {
	Filename string `json:"-"` // written as header since V2
	Beats []FrameN
	FrameRate int
	Staves []SavedStaff
	MasterGain float64 `json:",omitempty"`
	WaveGain float64 `json:",omitempty"`
	MidiGain float64 `json:",omitempty"`
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
		Mixer.LoadStaff(staff, sv)
	}
	sc.SetStaves(staves)
}

func round(x float64) float64 {
	return math.Floor(x + 0.5)
}

func convertFrames(f []FrameN, from, to int) {
	if from == 0 {
		from = 44100
	}
	if from == to {
		return
	}
	for i, _ := range f {
		f[i] = FrameN(round(float64(f[i])/float64(from) * float64(to)))
	}
}

func (s *stateV2) Capture(h *Headers) {
	h.Extra["Filename"] = G.audiofile
	s.FrameRate = audio.SampleRate
	s.Beats = G.score.BeatFrames()
	s.Staves = savedStaves(&G.score, s.Beats)
	s.MasterGain = Mixer.Master.Gain - 1.0
	s.WaveGain = Mixer.Wave.Gain - 1.0
	s.MidiGain = Mixer.Midi.Gain - 1.0
	s.MetronomeOff = Mixer.MuteMetronome
	s.WaveOff = Mixer.Wave.Muted
	s.MidiOff = Mixer.Midi.Muted
}

func (s *stateV2) Restore() {
	convertFrames(s.Beats, s.FrameRate, audio.SampleRate)
	G.score.LoadBeats(s.Beats)
	loadStaves(&G.score, s.Staves, s.Beats)
	Mixer.Master.Gain = s.MasterGain + 1.0
	Mixer.Wave.Gain = s.WaveGain + 1.0
	Mixer.Midi.Gain = s.MidiGain + 1.0
	Mixer.MuteMetronome = s.MetronomeOff
	Mixer.Wave.Muted = s.WaveOff
	Mixer.Midi.Muted = s.MidiOff
}

type Headers struct {
	Version int
	Extra map[string]interface{}
}

func mkHeaders() *Headers {
	return &Headers{currentVersion, make(map[string]interface{})}
}

func (h Headers) MarshalJSON() ([]byte, error) {
	var s string = fmt.Sprintf("{\"Version\": %d", h.Version)
	for k, v := range h.Extra {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		s += fmt.Sprintf(",\n\"%s\": %s", k, b)
	}
	s += "}\n"
	return []byte(s), nil
}

func (h *Headers) UnmarshalJSON(buf []byte) (err error) {
	var m map[string]interface{}
	err = json.Unmarshal(buf, &m)
	if err != nil {
		return err
	}
	h.Version = -1
	if i, ok := m["Version"]; ok {
		switch v := i.(type) {
		case float64:
			h.Version = int(v)
		}
		delete(m, "Version")
	}
	h.Extra = m
	if h.Version == -1 {
		return fmt.Errorf("missing required Version header. Have: %v", m)
	}
	return nil
}

var currentVersion = 2
// v2: moved Filename from data to header

func stateV(version int) state {
	switch version {
	case 1,2:
		return &stateV2{}
	}
	panic(fmt.Errorf("unknown file version %d", version))
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

func LoadState(filename string) (err error) {
	stateFile := fs.SaveDir() + "/" + key(filename)
	if _, err = os.Stat(stateFile); err == nil {
		var f *os.File
		f, err = os.Open(stateFile)
		defer mustRecover(&err)
		must(err)
		defer f.Close()
		j := json.NewDecoder(f)
		var h Headers
		must(j.Decode(&h))
		s := stateV(h.Version)
		must(j.Decode(&s))
		s.Restore()
	}
	return
}

func SaveState(filename string) (err error) {
	k := key(filename)
	tmpfile, err := ioutil.TempFile(fs.SaveDir(), k)
	defer mustRecover(&err)
	must(err)
	writeState(tmpfile)
	must(tmpfile.Close())
	must(fs.ReplaceFile(tmpfile.Name(), fs.SaveDir() + "/" + k))
	return nil
}

// panics on error
func writeState(tmpfile io.Writer) {
	s := stateV(currentVersion)
	h := mkHeaders()
	s.Capture(h)
	mustWrite(writeSlice(tmpfile, mustMarshal(h.MarshalJSON())))
	mustWrite(writeSlice(tmpfile, mustMarshal(json.MarshalIndent(s, "", "\t"))))
}

func writeSlice(w io.Writer, buf []byte) (int64, error) {
	return io.CopyN(w, bytes.NewReader(buf), int64(len(buf)))
}

func mustMarshal(buf []byte, err error) []byte {
	must(err)
	return buf
}

func mustWrite(n int64, err error) int64 {
	must(err)
	return n
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func mustRecover(err *error) {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			*err = e
		}
		panic(r)
	}
}

