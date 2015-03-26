package score

import (
	"math/big"
	"sort"

	"sqweek.net/sqribe/plumb"
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

func (score *Score) NewStaff(clef *Clef) *Staff {
	return &Staff{clef: clef, plumb: score.plumb}
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
		staff := &Staff{saved.Name, saved.Voice, saved.Velocity + 100, clef, KeySig(saved.Nsharps), saved.Muted, nil, score.plumb}
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

func (staff *Staff) RemoveNote(note *Note) {
	searchFn := func(i int)bool { return note.Cmp(staff.notes[i]) <= 0 }
	i := sort.Search(len(staff.notes), searchFn)
	if i == len(staff.notes) {
		return
	}
	if note.Cmp(staff.notes[i]) == 0 {
		copy(staff.notes[i:], staff.notes[i+1:])
		staff.notes = staff.notes[:len(staff.notes) - 1]
	}
	staff.plumb.C <- StaffChanged{staff}
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
		if note.Beat.frame > rng.Last.frame {
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

func OrderNotes(score *Score, notes chan<- StaffNote) {
	defer close(notes)
	n := len(score.staves)
	idx := make([]int, n)
	for j, staff := range score.staves {
		if staff.Muted {
			idx[j] = len(staff.notes)
		}
	}
	for {
		best := -1
		for j, staff := range(score.staves) {
			if idx[j] < len(staff.notes) {
				if best == -1 || staff.notes[idx[j]].Cmp(score.staves[best].notes[idx[best]]) < 0 {
					best = j
				}
			}
		}
		if best == -1 {
			break
		}
		notes <- StaffNote{score.staves[best], score.staves[best].notes[idx[best]]}
		idx[best]++
	}
}

