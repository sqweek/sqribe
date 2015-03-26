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
	InstEPiano = 5
	InstGuitar = 25
	InstEGuitar = 27
	InstViolin = 41
	InstVoice = 54
	InstWoodblock = 115
)

var degreeNames []string = []string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}

func PitchName(pitch uint8) string {
	degree := pitch % 12
	octave := pitch / 12
	return fmt.Sprintf("%s%d", degreeNames[degree], octave)
}

var instNames map[int]string
var instIds map[string]int

func inst(id int, name string) {
	instNames[id] = name
	instIds[name] = id
}

func init() {
	instNames = make(map[int]string)
	instIds = make(map[string]int)
	inst(InstPiano, "Piano")
	inst(InstEPiano, "E. Piano")
	inst(InstGuitar, "Guitar")
	inst(InstEGuitar, "E. Guitar")
	inst(InstViolin, "Violin")
	inst(InstVoice, "Voice")
	inst(InstWoodblock, "Woodblock")
}

func InstName(id int) string {
	name, ok := instNames[id]
	if ok {
		return name
	}
	return fmt.Sprintf("GM%03d", id)
}

func InstId(name string) int {
	id, ok := instIds[name]
	if ok {
		return id
	}
	return -1
}
