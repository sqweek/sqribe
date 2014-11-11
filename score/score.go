package score

import (
	"sync"
	"math/big"

	"sqweek.net/sqribe/plumb"

	. "sqweek.net/sqribe/core/types"
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
	plumb *plumb.Port

	quantApply chan chan bool
	quantCalc chan chan QuantizeBeats
}

type Measure struct {
	nbeats int /* length of measure */
	notes []Note /* sorted temporally */
}

func (score *Score) Init(plumb *plumb.Port) {
	score.staves = append(score.staves, score.NewTrebleStaff())
	score.staves = append(score.staves, score.NewBassStaff())
	score.beatLen = big.NewRat(1, 4)
	score.plumb = plumb
}

func (score *Score) Sub(origin interface{}, c chan interface{}) {
	score.plumb.Sub(origin, c)
}

func (score *Score) Unsub(origin interface{}) {
	score.plumb.Unsub(origin)
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
	//keys2d := make([]int, len(scale2degree), len(scale2degree))
	tone := -1
	for s, _ := range(scale2degree) {
		//keys2d[s] = scale2degree[s] + staff.accidental(s)
		if scale2degree[s] + staff.accidental(s) == degree {
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
	octave := 0
	d := int(pitch) - (int(staff.origin) - degree0)
	for d < 0 {
		octave -= 7
		d += 12
	}
	for d >= 12 {
		octave += 7
		d -= 12
	}
	delta := -tone0 + octave + tone
	return delta, nil
}
