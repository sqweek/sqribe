package midi

import (
	"fmt"
)

const (
	PitchD4 = 50
	PitchA4 = 57
	PitchC5 = 60
	PitchB5 = 71
	PitchF6 = 77
)
const (
	InstPiano = 0
	InstWoodblock = 115
)

var degreeNames []string = []string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}

func PitchName(pitch uint8) string {
	degree := pitch % 12
	octave := pitch / 12
	return fmt.Sprintf("%s%d", degreeNames[degree], octave)
}
