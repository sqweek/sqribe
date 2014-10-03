package main

import (
	"fmt"
)

const (
	pitchD4 = 50
	pitchB5 = 71
	pitchF6 = 77
)
const (
	instPiano = 0
	instWoodblock = 115
)

var degreeNames []string = []string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}

func PitchName(pitch uint8) string {
	degree := pitch % 12
	octave := pitch / 12
	return fmt.Sprintf("%s%d", degreeNames[degree], octave)
}
