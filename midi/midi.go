package midi

import (
	"fmt"
	"strconv"
	"strings"
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
	InstEPiano = 4
	InstGuitar = 25
	InstEGuitar = 27
	InstMuteGuitar = 28
	InstViolin = 40
	InstHarp = 46
	InstVoice = 53
	InstWoodblock = 115
)

var degreeNames []string = []string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}
var degreeNums map[string]int = map[string]int{"C": 0, "D": 2, "E": 4, "F": 5, "G": 7, "A": 9, "B": 11}

func PitchName(pitch uint8) string {
	degree := pitch % 12
	octave := pitch / 12
	return fmt.Sprintf("%s%d", degreeNames[degree], octave)
}

func eatAccidental(txt string) (int, string) {
	r := strings.SplitN(txt, "", 2)[0]
	switch r {
	case "b", "♭":
		return -1, txt[len(r):]
	case "#", "♯":
		return 1, txt[len(r):]
	}
	return 0, txt
}

type PitchError struct {
	name string
	msg string
}

func (e PitchError) Error() string {
	return fmt.Sprintf("invalid pitch: %s: %s", e.name, e.msg)
}

func ParsePitch(name string) (uint8, error) {
	if len(name) < 2 {
		return 255, &PitchError{name, "too short"}
	}
	degree, ok := degreeNums[name[0:1]]
	if !ok {
		return 255, &PitchError{name, "bad note"}
	}
	a, rest := eatAccidental(name[1:])
	octave, err := strconv.ParseUint(rest, 10, 8)
	if err != nil {
		return 255, &PitchError{name, fmt.Sprint("bad octave:", err)}
	}
	p := int(octave) * 12 + degree + a
	if p < 0 || p >= 128 {
		return 255, &PitchError{name, fmt.Sprintf("%d is outside midi range", p)}
	}
	return uint8(p), nil
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
	inst(InstMuteGuitar, "Muted Guitar")
	inst(InstViolin, "Violin")
	inst(InstHarp, "Harp")
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
