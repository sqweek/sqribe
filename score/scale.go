package score

import (
	"github.com/sqweek/sqribe/midi"
)

type KeySig int

type Clef struct {
	Name string
	Origin uint8 // unaltered midi pitch of center note
	tone int // tone index of center note (relative to C scale)
}

var TrebleClef Clef = Clef{"Treble", midi.PitchB5, 6}
var BassClef Clef = Clef{"Bass", midi.PitchD4, 1}
var AltoClef Clef = Clef{"Alto", midi.PitchC5, 0}
var TenorClef Clef = Clef{"Tenor", midi.PitchA4, 5}

var stdClef map[uint8]*Clef

func init() {
	stdClef = make(map[uint8]*Clef)
	for _, clef := range([]*Clef{&TrebleClef, &BassClef, &AltoClef}) {
		stdClef[clef.Origin] = clef
	}
}

func FindClef(origin uint8) *Clef {
	return stdClef[origin]
}

/* degrees: do di re ri mi fa fi so si la li ti
 * scales: C D E F G A B */
var degree2scale []int = []int{0, -1, 1, -1, 2, 3, -1, 4, -1, 5, -1, 6}
var scale2degree []int = []int{0, 2, 4, 5, 7, 9, 11}

var scaleSharps []KeySig = []KeySig{1, 3, 5, 0, 2, 4, 6}

// delta is the number of scale lines from the stave's center note. +ve = higher pitch
func (staff *Staff) PitchForLine(delta int) uint8 {
	return staff.clef.PitchForLine(staff.nsharps, delta)
}

func (clef *Clef) PitchForLine(nsharps KeySig, delta int) uint8 {
	pitch := int(clef.Origin)
	scale0 := clef.tone
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
	pitch += nsharps.accidental(s)
	return uint8(pitch)
}

func (nsharps KeySig) String() string {
	switch nsharps {
	case -7: return "Cb Major"
	case -6: return "Gb Major"
	case -5: return "Db Major"
	case -4: return "Ab Major"
	case -3: return "Eb Major"
	case -2: return "Bb Major"
	case -1: return "F Major"
	case 0: return "C Major"
	case 1: return "G Major"
	case 2: return "D Major"
	case 3: return "A Major"
	case 4: return "E Major"
	case 5: return "B Major"
	case 6: return "F# Major"
	case 7: return "C# Major"
	}
	return "???"
}

func (nsharps KeySig) IsSharps() bool {
	return nsharps > 0
}

func (nsharps KeySig) accidental(tone int) int {
	if nsharps > 0 && scaleSharps[tone] < nsharps {
		return 1
	} else if nsharps < 0 && scaleSharps[tone] - 6 > nsharps {
		return -1
	}
	return 0
}

/* returns n modulo d, on the range [0,d) */
func mod(n, d int) int {
	ans := n % d
	if ans < 0 {
		ans += d
	}
	return ans
}

func (key KeySig) toneForPitch(pitch uint8) int {
	degree := int(pitch % 12)
	for s, _ := range(scale2degree) {
		if mod(scale2degree[s] + key.accidental(s), 12) == degree {
			return s
		}
	}
	return -1
}

func lineWithAccidental(clef *Clef, nsharps KeySig, pitch uint8, dir int) (int, *int) {
	p := pitch + uint8(dir)
	tone := nsharps.toneForPitch(p)
	line, ok := lineForPitch(clef, nsharps, p)
	if !ok {
		return tone, nil
	}
	a := nsharps.accidental(tone) - dir
	return line, &a
}

func chooseAccidental(clef *Clef, key KeySig, pitch uint8) (int, *int) {
	// TODO consider other notes/accidentals in the bar/song
	flat, fax := lineWithAccidental(clef, key, pitch, 1)
	sharp, sax := lineWithAccidental(clef, key, pitch, -1)
	// at least one of ftone/stone is guaranteed to not be -1
	if key.IsSharps() && sax != nil && *sax != 2 {
		return sharp, sax
	} else if !key.IsSharps() && fax != nil && *fax != -2 {
		return flat, fax
	} else if fax == nil || (*fax == -2 && sax != nil) {
		return sharp, sax
	} else {
		return flat, fax
	}
}

/* raw pitch -> line conversion, no accidentals considered. */
func lineForPitch(clef *Clef, nsharps KeySig, pitch uint8) (int, bool) {
	tone := nsharps.toneForPitch(pitch)
	if tone == -1 {
		return 0, false
	}
	o := int(clef.Origin) + nsharps.accidental(int(clef.tone))
	Δp := int(pitch) - o
	Δt := tone - clef.tone
	delta := 7 * (Δp/12) + Δt
	/* toneForPitch doesn't include any octave information -- correct for those cases */
	if Δp < 0 && Δt > 0 {
		delta -= 7
	} else if Δp > 0 && Δt < 0 {
		delta += 7
	}
	return delta, true
}

func (staff *Staff) LineForPitch(pitch uint8) (int, *int) {
	return staff.clef.LineForPitch(staff.nsharps, pitch)
}

func (clef *Clef) LineForPitch(key KeySig, pitch uint8) (int, *int) {
	if delta, ok := lineForPitch(clef, key, pitch); ok {
		return delta, nil
	}
	return chooseAccidental(clef, key, pitch)
}

func (clef Clef) tones2lines(tones []int) []int {
	lines := make([]int, len(tones))
	for i := 0; i < len(tones); i++ {
		d := clef.tone - tones[i]
		if d <= -3 {
			d += 7
		} else if d >= 4 {
			d -= 7
		}
		lines[i] = d
	}
	return lines
}

/* pattern of accidental deltas, assuming Clef.tone is zero (C) */
func (nsharps KeySig) axpat() []int {
	if nsharps >= 0 {
		return []int{3, 0, 4, 1, -2, 2, -1}
	} else {
		return []int{-1, 2, -2, 1, -3, 0, -4}
	}
}

func (nsharps KeySig) Count() int {
	if nsharps >= 0 {
		return int(nsharps)
	}
	return int(-nsharps)
}

func (clef Clef) accidentalLines(nsharps KeySig) []int {
	lines := make([]int, 0, 7)
	diff := clef.tone
	if diff > 3 {
		diff -= 7
	}
	for i, deltaC := range nsharps.axpat() {
		if len(lines) >= nsharps.Count() {
			break
		}
		delta := deltaC - diff
		lim := 5
		if i == 0 {
			lim = 4
		}
		for delta > lim {
			delta -= 7
		}
		for delta < -lim {
			delta += 7
		}
		lines = append(lines, delta)
	}
	return lines
}
