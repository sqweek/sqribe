package main

import (
	"sort"
	"sync"
	"math"
	"math/big"
)

type BeatMap interface {
	// originally 'beat' was a big.Rat instead of float64, but it
	// just doesn't make sense to quantize individual beats
	ToFrame(beat float64) (FrameN, bool)
	ToBeat(frame FrameN) (float64, bool)
}

type Score struct {
	sync.RWMutex
	staves []*Staff
	beats []*BeatRef
	beatLen *big.Rat
}

type BeatRef struct {
	index int
	frame FrameN
}

type Staff struct {
	name string
	voice int
	origin uint8	// unaltered pitch of center note (ie. clef)
	nsharps int	// key signature (-ve for flats)
	Muted bool
	notes []*Note
}

type Measure struct {
	nbeats int /* length of measure */
	notes []Note /* sorted temporally */
}

type Note struct {
	Pitch uint8 /* midi pitch */
	Duration *big.Rat
	Beat *BeatRef
	Offset *big.Rat
}

func NewTrebleStaff() *Staff {
	return &Staff{name: "Treble", origin: 59}
}

func NewBassStaff() *Staff {
	return &Staff{name: "Bass", origin: 41}
}

func (score *Score) Init() {
	score.staves = append(score.staves, NewTrebleStaff())
	score.staves = append(score.staves, NewBassStaff())
	score.beatLen = big.NewRat(1, 4)
}

func (score *Score) ToFrame(beat float64) (FrameN, bool) {
	score.RLock()
	defer score.RUnlock()
	i := int(beat)
	α := beat - float64(i)
	if (α < 1e-6 && i + 1 == len(score.beats)) {
		return score.beats[i].frame, true
	}
	if i >= 0 && i + 1 < len(score.beats) {
		return FrameN((1 - α) * float64(score.beats[i].frame) + α * float64(score.beats[i+1].frame)), true
	}
	return -1, false
}

/* returns insertion index and true if the frame is already present */
func (score *Score) index(frame FrameN) (int, bool) {
	/* TODO binary search instead of linear */
	if len(score.beats) == 0 || frame < score.beats[0].frame {
		return 0, false
	}
	for i := 0; i < len(score.beats); i++ {
		if frame == score.beats[i].frame {
			return i, true
		} else if i + 1 >= len(score.beats) || frame < score.beats[i+1].frame {
			return i+1, false
		}
	}
	return len(score.beats), false
}

/* returns a fractional beat, and true if it is within the defined beat range */
func (score *Score) ToBeat(frame FrameN) (float64, bool) {
	score.RLock()
	defer score.RUnlock()
	if len(score.beats) == 0 || frame < score.beats[0].frame || frame > score.beats[len(score.beats)-1].frame {
		/* should perhaps extrapolate based on bpm... */
		return -1, false
	}
	i, exact := score.index(frame)
	if exact {
		return float64(i), true
	}
	α := float64(frame - score.beats[i-1].frame) / float64(score.beats[i].frame - score.beats[i-1].frame)
	return float64(i-1) + α, true
}

func (score *Score) BeatFrames() []FrameN {
	score.RLock()
	defer score.RUnlock()
	f := make([]FrameN, 0, len(G.score.beats))
	for i := 0; i < len(G.score.beats); i++ {
		f = append(f, G.score.beats[i].frame)
	}
	return f
}

func newBeat(index int, frame FrameN) *BeatRef {
	beat := new(BeatRef)
	beat.index = index
	beat.frame = frame
	return beat
}

func (score *Score) BeatIndex(beat *BeatRef) int {
	score.RLock()
	defer score.RUnlock()
	return beat.index
}

func (score *Score) LoadBeats(f []FrameN) {
	score.Lock()
	defer score.Unlock()
	if len(score.beats) > 0 {
		score.beats = score.beats[0:0]
	}
	for i := 0; i < len(f); i++ {
		score.beats = append(score.beats, newBeat(i, f[i]))
	}
}

func (score *Score) AddBeat(frame FrameN) {
	score.Lock()
	defer score.Unlock()
	if len(score.beats) == 0 {
		score.beats = append(score.beats, newBeat(0, frame))
		return
	}
	tolerance := FrameN(10000) //XXX should be based on sample rate/bpm
	i, exact := score.index(frame)
	if exact {
		return
	}
	if i > 0 && frame - score.beats[i-1].frame < tolerance {
		score.beats[i-1].frame = (score.beats[i-1].frame + frame) / 2
	} else if i == len(score.beats) {
		score.beats = append(score.beats, newBeat(len(score.beats), frame))
	} else if score.beats[i].frame - frame < tolerance {
		score.beats[i].frame = (score.beats[i].frame + frame) / 2
	} else {
		score.beats = append(score.beats, nil)
		copy(score.beats[i+1:], score.beats[i:])
		score.beats[i] = newBeat(i, frame)
		for j := i+1; j < len(score.beats); j++ {
			score.beats[j].index = j
		}
	}
}

func (score *Score) NearestBeat(frame FrameN) *BeatRef {
	score.RLock()
	defer score.RUnlock()
	if len(score.beats) == 0 {
		return nil
	}
	i, exact := score.index(frame)
	if exact || i == 0 {
		return score.beats[i]
	}
	if i == len(score.beats) {
		return score.beats[len(score.beats) - 1]
	}
	if frame - score.beats[i-1].frame < score.beats[i].frame - frame {
		return score.beats[i - 1]
	} else {
		return score.beats[i]
	}
}

func (score *Score) Quantize(beat float64) (*BeatRef, *big.Rat) {
	beati := int(beat)
	frac := beat - float64(beati)
	best := big.NewRat(0, 1)
	minErr := frac
	for _, i := range([]int{2, 3}) { // , 5}) { //, 7}) {
		for denom := int64(i); denom <= 8; denom <<= 1 {
			for num := int64(1); num < denom; num++ {
				r := big.NewRat(num, denom)
				/* TODO account for picked beats in error measure */
				f, _ := r.Float64()
				d := math.Abs(f - frac)
				if d < minErr {
					minErr = d
					best = r
				}
			}
		}
	}
	if 1 - frac < minErr {
		beati++
		best = big.NewRat(0, 1)
	}
	return score.beats[beati], best
}

func (score *Score) SavedStaves() []SavedStaff {
	score.RLock()
	defer score.RUnlock()
	saved := make([]SavedStaff, 0, len(score.staves))
	for _, staff := range score.staves {
		saved = append(saved, SavedStaff{staff.name, staff.voice, staff.origin, staff.nsharps, staff.Muted, staff.SavedNotes()})
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
	score.staves = score.staves[0:0]
	for _, saved := range in {
		staff := &Staff{saved.Name, saved.Voice, saved.Origin, saved.Nsharps, saved.Muted, nil}
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
}

func (staff *Staff) AddNote(note *Note) {
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

// 2 4 8 16 32 64 128
// 3 6 12 24 48 96
// 5 10 20 40 80
// 7 14 28 56 112

/* degrees: do di re ri mi fa fi so si la li ti
 * scales: C D E F G A B */
var degree2scale []int = []int{0, -1, 1, -1, 2, 3, -1, 4, -1, 5, -1, 6}
var scale2degree []int = []int{0, 2, 4, 5, 7, 9, 11}
var scaleSharps []int = []int{1, 3, 5, 0, 2, 4, 6}

// delta is the number of scale lines from the stave's center note. +ve = higher pitch
func (staff *Staff) PitchForLine(delta int) uint8 {
	pitch := int(staff.origin)
	scale0 := degree2scale[int(staff.origin % 12)]
	if scale0 == -1 {
		panic(staff.origin)
	}
	s := scale0 + delta
	/* first deal with octaves, in "scale" space */
	for s < 0 {
		pitch -= 12
		s += 7
	}
	for s >= 7 {
		pitch += 12
		s -= 7
	}
	/* then apply the intra-scale delta */
	pitch += scale2degree[s] - scale2degree[scale0]
	/* finally, apply the key signature */
	pitch += staff.accidental(s)
	return uint8(pitch)
}

func (staff *Staff) accidental(tone int) int {
	if staff.nsharps > 0 && scaleSharps[tone] < staff.nsharps {
		return 1
	} else if staff.nsharps < 0 && scaleSharps[tone] - 6 > staff.nsharps {
		return -1
	}
	return 0
}

func (staff *Staff) LineForPitch(pitch uint8) (int, *int) {
	degree0 := int(staff.origin % 12)
	tone0 := degree2scale[degree0]
	degree := int(pitch % 12)
	keys2d := make([]int, len(scale2degree), len(scale2degree))
	tone := -1
	for s, _ := range(scale2degree) {
		keys2d[s] = scale2degree[s] + staff.accidental(s)
		if keys2d[s] == degree {
			tone = s
		}
	}
	if tone == -1 {
		// pitch not in scale; use accidental
		// TODO consider other notes/accidentals in the bar/song
		delta, _ := staff.LineForPitch(pitch + 1)
		tone := ((tone0 + delta) % 7 + 7) % 7
		a := staff.accidental(tone) - 1
		if a == -2 {
			delta, _ = staff.LineForPitch(pitch - 1)
			tone = ((tone0 + delta) % 7 + 7) % 7
			a = staff.accidental(tone) + 1
		}
		return delta, &a
	}
	delta := -tone0 + 7 * ((int(pitch) - (int(staff.origin) - degree0)) / 12) + tone
	return delta, nil
}
