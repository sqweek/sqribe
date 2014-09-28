package main

import (
	"fmt"
)

var degreeNames []string = []string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}

func PitchName(pitch uint8) string {
	degree := pitch % 12
	octave := pitch / 12
	return fmt.Sprintf("%s%d", degreeNames[degree], octave)
}
