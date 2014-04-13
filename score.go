package main

import (
	"sort"
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
	staves []Staff
	beats []FrameN
	beatLen *big.Rat
	notes []*Note
}

type Staff struct {
	name string
	measures []Measure
	b0 int /* index of beat the measure starts on */
}

type Measure struct {
	nbeats int /* length of measure */
	notes []Note /* sorted temporally */
}

type Note struct {
	Pitch uint8 /* midi pitch */
	Duration *big.Rat
	Offset *big.Rat
}

func (score *Score) Init() {
	score.beatLen = big.NewRat(1, 4)
}

func (score *Score) ToFrame(beat float64) (FrameN, bool) {
	i := int(beat)
	α := beat - float64(i)
	if (α < 1e-6 && i + 1 == len(score.beats)) {
		return score.beats[i], true
	}
	if i + 1 < len(score.beats) {
		return FrameN((1 - α) * float64(score.beats[i]) + α * float64(score.beats[i+1])), true
	}
	return -1, false
}

/* returns insertion index and true if the frame is already present */
func (score *Score) index(frame FrameN) (int, bool) {
	/* TODO binary search instead of linear */
	if len(score.beats) == 0 || frame < score.beats[0] {
		return 0, false
	}
	for i := 0; i < len(score.beats); i++ {
		if frame == score.beats[i] {
			return i, true
		} else if i + 1 >= len(score.beats) || frame < score.beats[i+1] {
			return i+1, false
		}
	}
	return len(score.beats), false
}

/* returns a fractional beat, and true if it is within the defined beat range */
func (score *Score) ToBeat(frame FrameN) (float64, bool) {
	if len(score.beats) == 0 || frame < score.beats[0] || frame > score.beats[len(score.beats)-1] {
		/* should perhaps extrapolate based on bpm... */
		return -1, false
	}
	i, exact := score.index(frame)
	if exact {
		return float64(i), true
	}
	α := float64(frame - score.beats[i-1]) / float64(score.beats[i] - score.beats[i-1])
	return float64(i-1) + α, true
}

func (score *Score) AddBeat(frame FrameN) {
	if len(score.beats) == 0 {
		score.beats = append(score.beats, frame)
		return
	}
	tolerance := FrameN(20000) //XXX should be based on sample rate/bpm
	i, exact := score.index(frame)
	if exact {
		return
	}
	if i == 0 {
		score.beats = append(score.beats, 0)
		copy(score.beats[1:], score.beats[0:])
		score.beats[0] = frame
	} else if frame - score.beats[i-1] < tolerance {
		score.beats[i-1] = (score.beats[i-1] + frame) / 2
	} else if i == len(score.beats) {
		score.beats = append(score.beats, frame)
	} else if score.beats[i] - frame < tolerance {
		score.beats[i] = (score.beats[i] + frame) / 2
	} else {
		score.beats = append(score.beats, 0)
		copy(score.beats[i+1:], score.beats[i:])
		score.beats[i] = frame
	}
}

func (score *Score) NearestBeat(frame FrameN) FrameN {
	if len(score.beats) == 0 {
		return 0
	}
	i, exact := score.index(frame)
	if exact || i == 0 {
		return score.beats[i]
	}
	if i == len(score.beats) {
		return score.beats[len(score.beats) - 1]
	}
	if frame - score.beats[i-1] < score.beats[i] - frame {
		return score.beats[i - 1]
	} else {
		return score.beats[i]
	}
}

func (score *Score) Quantize(beat float64) (int, *big.Rat) {
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
	best.Mul(best, score.beatLen)
	return beati, best
}

func (note *Note) Set(note2 *Note) {
	note.Pitch = note2.Pitch
	note.Offset.Set(note2.Offset)
	note.Duration.Set(note2.Duration)
}

func (note *Note) Cmp(note2 *Note) int {
	d := note.Offset.Cmp(note2.Offset)
	if d == 0 {
		return int(note.Pitch - note2.Pitch)
	}
	return d
}

func (score *Score) AddNote(note *Note) {
	if len(score.notes) == 0 {
		score.notes = append(score.notes, note)
		return
	}
	searchFn := func(i int)bool { return note.Cmp(score.notes[i]) <= 0 }
	i := sort.Search(len(score.notes), searchFn)
	if i == len(score.notes) {
		score.notes = append(score.notes, note)
	} else if note.Cmp(score.notes[i]) == 0 {
		/* already have a note at this offset with the same pitch, update the duration */
		score.notes[i].Duration.Set(note.Duration)
	} else {
		score.notes = append(score.notes, nil)
		copy(score.notes[i+1:], score.notes[i:])
		score.notes[i] = note
	}
}

// 2 4 8 16 32 64 128
// 3 6 12 24 48 96
// 5 10 20 40 80
// 7 14 28 56 112
