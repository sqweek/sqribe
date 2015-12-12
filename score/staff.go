package score

import (
	"math/big"
	"sort"

	. "sqweek.net/sqribe/core/types"
)

type Staff struct {
	name string
	clef *Clef
	nsharps KeySig	// key signature (-ve for flats)
	notes []*Note
}

type Note struct {
	Pitch uint8 /* midi pitch */
	Duration *big.Rat
	Beat *BeatRef
	Offset *big.Rat
}

type StaffChanged struct {
	Staves map[*Staff]struct{}
}

type KeyChanged StaffChanged

func staffChanged(staves... *Staff) StaffChanged {
	c := StaffChanged{make(map[*Staff]struct{})}
	for _, staff := range staves {
		c.Staves[staff] = struct{}{}
	}
	return c
}

func notesChanged(notes []StaffNote) StaffChanged {
	c := StaffChanged{make(map[*Staff]struct{})}
	for _, sn := range notes {
		if _, ok := c.Staves[sn.Staff]; !ok {
			c.Staves[sn.Staff] = struct{}{}
		}
	}
	return c
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
	}
	score.plumb.C <- KeyChanged(staffChanged(score.staves...))
}

func (score *Score) Staves() []*Staff {
	return score.staves
}

func MkStaff(name string, clef *Clef, key KeySig) *Staff {
	return &Staff{name: name, clef: clef, nsharps: key}
}

/* XXX should probably be an Op but only used during startup for now */
func (score *Score) SetStaves(staves []*Staff) {
	score.staves = staves
	score.plumb.C <- staffChanged(staves...)
}

type AddStaffOp struct {
	staff *Staff
}

func (score *Score) AddStaff(staff *Staff) {
	score.update(&AddStaffOp{staff})
	score.plumb.C <- staffChanged(staff)
}

func (op *AddStaffOp) apply(score *Score) bool {
	score.staves = append(score.staves, op.staff)
	return true
}

func (op *AddStaffOp) undo(score *Score) {
	score.staves = score.staves[:len(score.staves)-1]
}

func (score *Score) Beatf(note *Note) BeatPoint {
	f, _ := note.Offset.Float64()
	return BeatPt{note.Beat, f}
}

func (score *Score) EndBeatf(note *Note) BeatPoint {
	var r big.Rat
	r.Set(note.Offset)
	r.Add(&r, note.Duration)
	f, _ := r.Float64()
	b := note.Beat
	for f > 1 {
		b = b.LNext()
		f -= 1
	}
	return BeatPt{b, f}
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

func (src *Note) Dup() (dst *Note) {
	dst = &Note{Offset: new(big.Rat), Duration: new(big.Rat)}
	dst.Beat = src.Beat
	dst.Offset.Set(src.Offset)
	dst.Pitch = src.Pitch
	dst.Duration.Set(src.Duration)
	return dst
}

func (staff *Staff) removeNote(note *Note) bool {
	searchFn := func(i int)bool { return note.Cmp(staff.notes[i]) <= 0 }
	i := sort.Search(len(staff.notes), searchFn)
	if i < len(staff.notes) && note == staff.notes[i] {
		copy(staff.notes[i:], staff.notes[i+1:])
		staff.notes = staff.notes[:len(staff.notes) - 1]
		return true
	}
	return false
}

func (score *Score) AddNotes(staff *Staff, notes... *Note) {
	score.update(&AddNotesOp{staff, notes})
	score.plumb.C <- staffChanged(staff)
}

type AddNotesOp struct {
	staff *Staff
	notes []*Note
}

func (op *AddNotesOp) apply(score *Score) bool {
	op.staff.addNote(op.notes...)
	return true
}

func (op *AddNotesOp) undo(score *Score) {
	for _, note := range op.notes {
		if !op.staff.removeNote(note) {
			//XXX need to restore original duration
		}
	}
}

func (staff *Staff) addNote(note... *Note) {
	staff.notes = Merge(staff.notes, note...)
}

/* Merges 'notes' into the already-sorted 'list'. */
func Merge(list []*Note, notes... *Note) []*Note {
	if len(list) == 0 {
		list = append(list, notes...)
		return list
	}
	for _, note := range notes {
		searchFn := func(i int)bool { return note.Cmp(list[i]) <= 0 }
		i := sort.Search(len(list), searchFn)
		if i == len(list) {
			list= append(list, note)
		} else if note.Cmp(list[i]) == 0 {
			/* already have a note at this offset with the same pitch, update the duration */
			list[i].Duration.Set(note.Duration)
		} else {
			list = append(list, nil)
			copy(list[i+1:], list[i:])
			list[i] = note
		}
	}
	return list
}

func (score *Score) MvNotes(Δpitch int8, Δbeat *big.Rat, notes... StaffNote) {
	score.update(&MvNotesOp{Δpitch, Δbeat, notes})
	score.plumb.C <- notesChanged(notes)
}

type MvNotesOp struct {
	Δpitch int8
	Δbeat *big.Rat
	notes []StaffNote
}

func (op *MvNotesOp) apply(score *Score) bool {
	op.mv(op.Δpitch, op.Δbeat)
	return true
}

func (op *MvNotesOp) mv(Δpitch int8, Δbeat *big.Rat) {
	for _, sn := range op.notes {
		sn.Staff.removeNote(sn.Note)
	}
	for _, sn := range op.notes {
		sn.Note.Mv(Δpitch, Δbeat)
		sn.Staff.addNote(sn.Note)
	}
}

func (op *MvNotesOp) undo(score *Score) {
	// XXX if addNote modified Duration of any notes, that is not restored
	op.mv(-op.Δpitch, (&big.Rat{}).Neg(op.Δbeat))
}

// needs to clip resulting pitch/beat
func (note *Note) Mv(Δpitch int8, Δbeat *big.Rat) *Note {
	note.Pitch += uint8(Δpitch)
	note.Offset.Add(note.Offset, Δbeat)
	f, _ := note.Offset.Float64()
	for f > 1.0 {
		note.Beat = note.Beat.LNext()
		f -= 1.0;
		note.Offset.Sub(note.Offset, big.NewRat(1, 1))
	}
	for f < 0.0 {
		note.Beat = note.Beat.LPrev()
		f += 1.0;
		note.Offset.Add(note.Offset, big.NewRat(1, 1))
	}
	return note
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
	op := &RepeatNotesOp{rng: rng}
	score.update(op)
	score.plumb.C <- notesChanged(op.added)
}

type RepeatNotesOp struct {
	rng BeatRange
	added []StaffNote
}

func (op *RepeatNotesOp) apply(score *Score) bool {
	rng := op.rng
	dest := op.rng.Last
	n := rng.Last.Subtract(rng.First)
	if extra := rng.Last.Walk(n).Subtract(rng.Last); extra < n {
		/* truncate the source range so we don't go past the defined beats */
		rng = BeatRange{rng.First, rng.Last.Walk(extra - n)}
	}
	next := score.Iter(rng)
	var src StaffNote
	for next != nil {
		src, next = next()
		note := src.Note.Dup()
		note.Beat = dest.Walk(note.Beat.Subtract(rng.First))
		src.Staff.addNote(note)
		op.added = append(op.added, StaffNote{src.Staff, note})
	}
	return true
}

func (op *RepeatNotesOp) undo(score *Score) {
	// XXX if addNote modified the durations of any existing notes that is not undone
	for _, sn := range op.added {
		sn.Staff.removeNote(sn.Note)
	}
}

func (score *Score) RemoveNotes(notes... StaffNote) {
	score.update(&RemoveNotesOp{notes})
	score.plumb.C <- notesChanged(notes)
}

type RemoveNotesOp struct {
	notes []StaffNote
}

func (op *RemoveNotesOp) apply(score *Score) bool {
	for _, sn := range op.notes {
		sn.Staff.removeNote(sn.Note)
	}
	return len(op.notes) > 0
}

func (op *RemoveNotesOp) undo(score *Score) {
	for _, sn := range op.notes {
		sn.Staff.addNote(sn.Note)
	}
}

func (staff *Staff) Name() string {
	return staff.name
}

func (staff *Staff) Clef() *Clef {
	return staff.clef
}

func (staff *Staff) Key() KeySig {
	return staff.nsharps
}

func (staff *Staff) Notes() []*Note {
	return staff.notes
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
