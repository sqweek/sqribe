package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"strings"

	"github.com/sqweek/sqribe/audio"
	"github.com/sqweek/sqribe/log"
	"github.com/sqweek/sqribe/midi"
	"github.com/sqweek/sqribe/score"

	. "github.com/sqweek/sqribe/core/types"
)

type SavedStaff struct {
	Name string
	Voice int
	Velocity int
	Origin uint8
	Nsharps int
	Muted bool `json:",omitempty"`
	Notes []SavedNote `json:",omitempty"` // use Notestr since V3
	Notestr []string

	Minimised bool `json:",omitempty"`
}

type SavedNote struct {
	Pitch uint8
	Duration *big.Rat
	Offset *big.Rat
}

type SavedView struct {
	Width int
	Height int
}

type State interface {
	Headers() *Headers
	View() SavedView
	Restore() // restores this objects state to the memory model

	Write(io.Writer) error
}

type stateV3 struct {
	h *Headers `json:"-"` // serialised separately

	Filename string `json:",omitempty"` // written as header since V2
	Beats []FrameN
	FrameRate int
	Staves []SavedStaff
	Tuning float64 `json:",omitempty"`
	MasterGain float64 `json:",omitempty"`
	WaveGain float64 `json:",omitempty"`
	MidiGain float64 `json:",omitempty"`
	MetronomeOff bool `json:",omitempty"`
	WaveOff bool `json:",omitempty"`
	MidiOff bool `json:",omitempty"`
	Pos struct {
		First FrameN
		Zoom int
	}
	Win SavedView
}

func savedNotes(staff *score.Staff, beats []FrameN) []string {
	notes := staff.Notes()
	saved := make([]string, 0, len(notes))
	i := 0
	for _, note := range notes {
		for beats[i] < note.Beat.Frame() {
			i++
		}
		b := big.NewRat(int64(i), 1)
		b.Add(b, note.Offset)
		saved = append(saved, fmt.Sprintf("%s %v %v", midi.PitchName(note.Pitch), note.Duration, b))
	}
	return saved
}

func loadNotes(sc *score.Score, staff *score.Staff, n int, notefn noteFunc, beats []FrameN) []*score.Note {
	notes := make([]*score.Note, 0, n)
	beat := sc.Head
	for i := 0; i < n; i++ {
		pitch, duration, offset, err := notefn(i)
		if err != nil {
			log.FS.Printf("error loading note %d: %v\n", i, err)
			continue
		}
		beatf, _ := offset.Float64()
		bi := int(beatf)
		for beat.Frame() < beats[bi] {
			beat = beat.Next()
		}
		offset.Sub(offset, big.NewRat(int64(bi), 1))
		notes = append(notes, &score.Note{pitch, duration, beat, offset})
	}
	return notes
}

func savedStaves(score *score.Score, beats []FrameN) []SavedStaff {
	staves := score.Staves()
	saved := make([]SavedStaff, 0, len(staves))
	for _, staff := range staves {
		notes := savedNotes(staff, beats)
		mix := Mixer.For(staff)
		saved = append(saved, SavedStaff{staff.Name(), mix.Voice, mix.Velocity - 100, staff.Clef().Origin, int(staff.Key()), mix.Muted, nil, notes, G.ww.IsMinimised(staff)})
	}
	return saved
}

type noteFunc func(int)(uint8, *big.Rat, *big.Rat, error)

func noteFnFromStrings(notes []string) noteFunc {
	return func(i int)(pitch uint8, dur, off *big.Rat, err error) {
		f := strings.Split(notes[i], " ")
		dur = big.NewRat(-1, 1)
		off = big.NewRat(-1, 1)
		if pitch, err = midi.ParsePitch(f[0]); err == nil {
			if _, ok := dur.SetString(f[1]); ok {
				if _, ok := off.SetString(f[2]); !ok {
					err = fmt.Errorf("note '%s': bad offset", notes[i])
				}
				return
			} else {
				err = fmt.Errorf("note '%s': bad duration", notes[i])
			}
		}
		return
	}
}

func noteFnFromStructs(notes []SavedNote) noteFunc {
	return func(i int)(pitch uint8, dur, off *big.Rat, err error) {
		n := notes[i]
		return n.Pitch, n.Duration, n.Offset, nil
	}
}

// XXX lots of pointless change events while building staves
func loadStaves(sc *score.Score, saved []SavedStaff, beats []FrameN) {
	minimised := make(map[*score.Staff]bool)
	staves := make([]*score.Staff, 0, len(saved))
	for _, sv := range saved {
		clef := score.FindClef(sv.Origin)
		if clef == nil {
			clef = &score.TrebleClef
		}
		staff := score.MkStaff(sv.Name, clef, score.KeySig(sv.Nsharps))
		var n int
		var notefn noteFunc
		if len(sv.Notestr) > 0 {
			n = len(sv.Notestr)
			notefn = noteFnFromStrings(sv.Notestr)
		} else {
			n = len(sv.Notes)
			notefn = noteFnFromStructs(sv.Notes)
		}
		sc.AddNotes(staff, loadNotes(sc, staff, n, notefn, beats)...)
		staves = append(staves, staff)
		Mixer.LoadStaff(staff, sv)
		minimised[staff] = sv.Minimised
	}
	sc.SetStaves(staves)
	G.ww.RestoreStaffView(minimised)
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

// captures current memory model state
func CaptureState() State {
	s := stateV(mkHeaders())
	s.h.Extra["Filename"] = G.files.Audio
	s.FrameRate = audio.SampleRate
	s.Beats = G.score.BeatFrames()
	s.Staves = savedStaves(G.score, s.Beats)
	s.Tuning = Synth.Tuning()
	s.MasterGain = Mixer.Master.Gain - 1.0
	s.WaveGain = Mixer.Wave.Gain - 1.0
	s.MidiGain = Mixer.Midi.Gain - 1.0
	s.MetronomeOff = Mixer.MuteMetronome
	s.WaveOff = Mixer.Wave.Muted
	s.MidiOff = Mixer.Midi.Muted
	s.Pos.First, s.Pos.Zoom = G.ww.CapturePos()
	s.Win.Width, s.Win.Height = RootWin.Size()
	return s
}

func EmptyState() State {
	return stateV(mkHeaders())
}

func (s *stateV3) Headers() *Headers {
	return s.h
}

func (s *stateV3) View() SavedView {
	return s.Win
}

func (s *stateV3) Restore() {
	convertFrames(s.Beats, s.FrameRate, audio.SampleRate)
	G.score.LoadBeats(s.Beats)
	loadStaves(G.score, s.Staves, s.Beats)
	Synth.SetTuning(s.Tuning)
	Mixer.Master.Gain = s.MasterGain + 1.0
	Mixer.Wave.Gain = s.WaveGain + 1.0
	Mixer.Midi.Gain = s.MidiGain + 1.0
	Mixer.MuteMetronome = s.MetronomeOff
	Mixer.Wave.Muted = s.WaveOff
	Mixer.Midi.Muted = s.MidiOff
	if (s.Pos.Zoom != 0) {
		G.ww.RestorePos(s.Pos.First, s.Pos.Zoom)
	}
	/* s.Win is restored separately */
}

type Headers struct {
	Version int
	Extra map[string]interface{}
}

func mkHeaders() *Headers {
	return &Headers{currentVersion, make(map[string]interface{})}
}

func (h Headers) String(key string) string {
	if v, ok := h.Extra[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
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

var currentVersion = 3
// v2: moved Filename from data to header
// v3: save notes as strings not structs

func stateV(h *Headers) *stateV3 {
	switch h.Version {
	case 1, 2, 3:
		return &stateV3{h: h}
	}
	panic(fmt.Errorf("unknown file version %d", h.Version))
}

func ReadState(f io.Reader) (s State, err error) {
	defer mustRecover(&err)
	j := json.NewDecoder(f)
	var h Headers
	must(j.Decode(&h))
	sv := stateV(&h)
	must(j.Decode(&sv))
	if _, ok := h.Extra["Filename"]; !ok {
		// v1 didn't have Filename in Headers
		h.Extra["Filename"] = sv.Filename
	}
	s = sv
	return
}

func (s *stateV3) Write(tmpfile io.Writer) (err error) {
	defer mustRecover(&err)
	// explicitly calling Headers.MarshalJSON to preserve whitespace (json.Marshal would collapse it)
	mustWrite(writeSlice(tmpfile, mustMarshal(s.h.MarshalJSON())))
	mustWrite(writeSlice(tmpfile, mustMarshal(json.MarshalIndent(s, "", "\t"))))
	return
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

