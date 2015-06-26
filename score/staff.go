package score

import (
	"math/big"
	"sort"

	"sqweek.net/sqribe/midi"
	"sqweek.net/sqribe/plumb"

	. "sqweek.net/sqribe/core/types"
)

type Staff struct {
	name string
	voice int
	velocity int
	clef *Clef
	nsharps KeySig	// key signature (-ve for flats)
	Muted bool
	notes []*Note
	plumb *plumb.Port

	Minimised bool // should probably live elsewhere...
}

type Note struct {
	Pitch uint8 /* midi pitch */
	Duration *big.Rat
	Beat *BeatRef
	Offset *big.Rat
}

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

type StaffChanged struct {
	Staff *Staff
}

type StaffNote struct {
	Staff *Staff
	Note *Note
}

func (score *Score) Key() KeySig {
	if len(score.staves) == 0 {
		return 0
	}
	return score.staves[0].nsharps
}

func (score *Score) KeyChange(dsharps int) {
	for _, staff := range score.staves {
		staff.nsharps += KeySig(dsharps)
		if staff.nsharps > 7 {
			staff.nsharps -= 12
		} else if staff.nsharps < -7 {
			staff.nsharps += 12
		}
		staff.plumb.C <- StaffChanged{staff}
	}
}

func (score *Score) Staves() []*Staff {
	return score.staves
}

func (score *Score) AddStaff(clef *Clef) {
	nsharps := KeySig(0)
	if len(score.staves) > 0 {
		nsharps = score.staves[0].nsharps
	}
	staff := &Staff{clef: clef, voice: midi.InstPiano, velocity: 100, nsharps: nsharps, plumb: score.plumb}
	score.staves = append(score.staves, staff)
	score.plumb.C <- StaffChanged{staff}
}

func (score *Score) SavedStaves() []SavedStaff {
	score.RLock()
	defer score.RUnlock()
	saved := make([]SavedStaff, 0, len(score.staves))
	for _, staff := range score.staves {
		saved = append(saved, SavedStaff{staff.name, staff.voice, staff.velocity - 100, staff.clef.Origin, int(staff.nsharps), staff.Muted, staff.SavedNotes()})
	}
	return saved
}

func (staff *Staff) SavedNotes() []SavedNote {
	out := make([]SavedNote, 0, len(staff.notes))
	for _, note := range staff.notes {
		b := big.NewRat(int64(note.Beat.index), 1)
		b.Add(b, note.Offset)
		out = append(out, SavedNote{note.Pitch, note.Duration, b})
	}
	return out
}

func (score *Score) LoadStaves(in []SavedStaff) {
	if len(score.staves) > 0 {
		score.staves = score.staves[0:0]
	}
	for _, saved := range in {
		clef := FindClef(saved.Origin)
		if clef == nil {
			clef = &TrebleClef
		}
		staff := &Staff{saved.Name, saved.Voice, saved.Velocity + 100, clef, KeySig(saved.Nsharps), saved.Muted, nil, score.plumb, false}
		staff.LoadNotes(score, saved.Notes)
		score.staves = append(score.staves, staff)
	}
}

func (staff *Staff) LoadNotes(score *Score, in []SavedNote) {
	if len(staff.notes) > 0 {
		staff.notes = staff.notes[0:0]
	}
	for _, saved := range in {
		beatf, _ := saved.Offset.Float64()
		beati := int(beatf)
		saved.Offset.Sub(saved.Offset, big.NewRat(int64(beati), 1))
		note := &Note{saved.Pitch, saved.Duration, score.beats[beati], saved.Offset}
		staff.AddNote(note)
	}
	staff.plumb.C <- StaffChanged{staff}
}

func (score *Score) Beatf(note *Note) float64 {
	score.RLock()
	defer score.RUnlock()
	b := big.NewRat(int64(note.Beat.index), 1)
	b.Add(b, note.Offset)
	f, _ := b.Float64()
	return f
}

func (note *Note) Durf() float64 {
	d, _ := note.Duration.Float64()
	return d;
}

func (note *Note) Cmp(note2 *Note) int {
	if note.Beat.frame < note2.Beat.frame {
		return -1
	} else if note.Beat.frame > note2.Beat.frame {
		return 1
	}
	d := note.Offset.Cmp(note2.Offset)
	if d == 0 {
		return int(note.Pitch) - int(note2.Pitch)
	}
	return d
}

func (note *Note) Set(note2 *Note) {
	note.Beat = note2.Beat
	note.Offset.Set(note2.Offset)
	note.Pitch = note2.Pitch
	note.Duration.Set(note2.Duration)
}

func (staff *Staff) RemoveNote(note *Note) bool {
	removed := staff.removeNote(note)
	staff.plumb.C <- StaffChanged{staff}
	return removed
}

func (staff *Staff) removeNote(note *Note) bool {
	searchFn := func(i int)bool { return note.Cmp(staff.notes[i]) <= 0 }
	i := sort.Search(len(staff.notes), searchFn)
	if i == len(staff.notes) {
		return false
	}
	if note.Cmp(staff.notes[i]) == 0 {
		copy(staff.notes[i:], staff.notes[i+1:])
		staff.notes = staff.notes[:len(staff.notes) - 1]
		return true
	}
	return false
}

func (staff *Staff) AddNote(note *Note) {
	staff.addNote(note)
	staff.plumb.C <- StaffChanged{staff}
}

func (staff *Staff) addNote(note *Note) {
	if len(staff.notes) == 0 {
		staff.notes = append(staff.notes, note)
		return
	}
	searchFn := func(i int)bool { return note.Cmp(staff.notes[i]) <= 0 }
	i := sort.Search(len(staff.notes), searchFn)
	if i == len(staff.notes) {
		staff.notes = append(staff.notes, note)
	} else if note.Cmp(staff.notes[i]) == 0 {
		/* already have a note at this offset with the same pitch, update the duration */
		staff.notes[i].Duration.Set(note.Duration)
	} else {
		staff.notes = append(staff.notes, nil)
		copy(staff.notes[i+1:], staff.notes[i:])
		staff.notes[i] = note
	}
}

func (score *Score) MvNotes(Δpitch uint8, Δbeat float64, notes... StaffNote) {
	score.RLock()
	changed := make(map[*Staff]struct{})
	for _, sn := range notes {
		sn.Staff.removeNote(sn.Note)
	}
	for _, sn := range notes {
		sn.Note.Pitch += Δpitch
		b := score.Beatf(sn.Note) + Δbeat
		beat, offset := score.Quantize(b)
		sn.Note.Beat = beat
		sn.Note.Offset.Set(offset)
		sn.Staff.addNote(sn.Note)
		changed[sn.Staff] = struct{}{}
	}
	score.RUnlock()
	for staff, _ := range changed {
		staff.plumb.C <- StaffChanged{staff}
	}
}

/* Ignores Duration field of supplied Note. */
func (staff *Staff) NoteAt(note *Note) *Note {
	searchFn := func(i int)bool { return note.Cmp(staff.notes[i]) <= 0 }
	i := sort.Search(len(staff.notes), searchFn)
	if i < len(staff.notes) && note.Cmp(staff.notes[i]) == 0 {
		return staff.notes[i]
	}
	return nil
}

func (score *Score) RepeatNotes(rng BeatRange) {
	if rng.First == rng.Last {
		return
	}
	score.RLock()
	n := rng.Last.index - rng.First.index
	if extra := rng.Last.index + n - len(score.beats); extra > 0 {
		/* truncate the source range so we don't go past the defined beats */
		rng = BeatRange{rng.First, score.beats[rng.Last.index - extra]}
	}
	affectedStaves := make(map[*Staff]bool)
	repeatNote := func (staff *Staff, note *Note) {
		note2 := Note{note.Pitch, note.Duration, score.beats[note.Beat.index + n], note.Offset}
		staff.addNote(&note2)
		affectedStaves[staff] = true
	}
	score.perStaffNote(rng, repeatNote)
	score.RUnlock()
	for staff, _ := range affectedStaves {
		staff.plumb.C <- StaffChanged{staff}
	}
}


func (score *Score) RemoveNotes(rng BeatRange) {
	affectedStaves := make(map[*Staff]bool)
	f := func(staff *Staff, note *Note) {
		staff.RemoveNote(note)
		affectedStaves[staff] = true
	}
	score.perStaffNote(rng, f)
	for staff, _ := range affectedStaves {
		staff.plumb.C <- StaffChanged{staff}
	}
}

func (score *Score) perStaffNote(rng BeatRange, f func(staff *Staff, note *Note)) {
	for i := 0; i < len(score.staves); i++ {
		staff := score.staves[i]
		if !staff.Muted {
			staff.perNote(rng, func(note *Note) {f(staff, note)})
		}
	}
}

func (staff *Staff) perNote(rng BeatRange, f func(note *Note)) {
	searchFn := func(i int)bool { return rng.First.frame <= staff.notes[i].Beat.frame }
	selectedNotes := make([]*Note, 0, 16)
	for i := sort.Search(len(staff.notes), searchFn); i < len(staff.notes); i++ {
		note := staff.notes[i]
		if note.Beat.frame >= rng.Last.frame {
			break
		}
		selectedNotes = append(selectedNotes, note)
	}
	for _, note := range selectedNotes {
		f(note)
	}
}

func (staff *Staff) Name() string {
	return staff.name
}

func (staff *Staff) Voice() int {
	return staff.voice
}

func (staff *Staff) Velocity() int {
	return staff.velocity
}

func (staff *Staff) Notes() []*Note {
	return staff.notes
}

func (staff *Staff) SetVoice(voice int) {
	staff.voice = voice
	staff.plumb.C <- StaffChanged{staff}
}

func (staff *Staff) SetVelocity(velocity int) {
	staff.velocity = velocity
	staff.plumb.C <- StaffChanged{staff}
}

func (staff *Staff) ToggleMute() {
	staff.Muted = !staff.Muted
	staff.plumb.C <- StaffChanged{staff}
}

func (staff *Staff) KeyAccidentalLines() (KeySig, []int) {
	return staff.nsharps, staff.clef.accidentalLines(staff.nsharps)
}

type NoteIter func()(StaffNote, NoteIter)

func (score *Score) Iter(rng TimeRange, staves... *Staff) NoteIter {
	if staves == nil {
		staves = score.staves
	}
	idx := make([]int, len(staves))
	toFrame := func(note *Note)FrameN {
		f, _ := score.ToFrame(score.Beatf(note))
		return f
	}
	for j, staff := range staves {
		searchFn := func(i int)bool { return rng.MinFrame() <= toFrame(staff.notes[i]) }
		idx[j] = sort.Search(len(staff.notes), searchFn)
	}
	bestFn := func(idx []int, score *Score)int {
		best := -1
		for j, staff := range(staves) {
			if idx[j] < len(staff.notes) && toFrame(staff.notes[idx[j]]) < rng.MaxFrame() {
				if best == -1 || staff.notes[idx[j]].Cmp(staves[best].notes[idx[best]]) < 0 {
					best = j
				}
			}
		}
		return best
	}
	nxt := bestFn(idx, score)
	if nxt == -1 {
		return nil
	}
	var fn NoteIter
	fn = func()(StaffNote, NoteIter) {
		best := nxt
		inote := idx[best]
		idx[best]++
		nxt = bestFn(idx, score)
		if nxt == -1 {
			fn = nil
		}
		return StaffNote{staves[best], staves[best].notes[inote]}, fn
	}
	return fn
}

type ChordIter func()([]StaffNote, ChordIter)

func Chords(notes NoteIter) ChordIter {
	if notes == nil {
		return nil
	}
	sn, nextNote := notes()
	chord := []StaffNote{sn}
	var nextChord ChordIter
	nextChord = func()([]StaffNote, ChordIter) {
		for nextNote != nil {
			sn, nextNote = nextNote()
			if chord[0].Note.Beat == sn.Note.Beat && chord[0].Note.Offset.Cmp(sn.Note.Offset) == 0 {
				chord = append(chord, sn)
			} else {
				result := chord
				chord = []StaffNote{sn}
				return result, nextChord
			}
		}
		return chord, nil
	}
	return nextChord
}
