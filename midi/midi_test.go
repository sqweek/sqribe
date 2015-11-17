package midi

import (
	"testing"
)

func TestPitchNames(t *testing.T) {
	for i := uint8(0); i < 128; i++ {
		name := PitchName(i)
		p, err := ParsePitch(name)
		if err != nil {
			t.Fatalf("Pitch(%s) error: %v", name, err)
		}
		if p != i {
			t.Fatalf("TestPitchNames: %d => %s => %d", i, name, p)
		}
	}
}

func parse(name string) uint8 {
	p, err := ParsePitch(name)
	if err != nil {
		panic(err)
	}
	return p
}

func pitchMustEqual(t *testing.T, name string, expected uint8) {
	if parse(name) != expected {
		t.Fatalf("Pitch %s: expected %d but got %d", name, expected, parse(name))
	}
}

func TestKnown(t *testing.T) {
	pitchMustEqual(t, "D4", PitchD4)
	pitchMustEqual(t, "A4", PitchA4)
	pitchMustEqual(t, "C5", PitchC5)
	pitchMustEqual(t, "B5", PitchB5)
	pitchMustEqual(t, "F6", PitchF6)
	pitchMustEqual(t, "F♯6", PitchF6+1)
	pitchMustEqual(t, "F#6", PitchF6+1)
	pitchMustEqual(t, "B♭5", PitchB5-1)
	pitchMustEqual(t, "Bb5", PitchB5-1)
}

func pitchMustFail(t *testing.T, name string) {
	p, err := ParsePitch(name)
	if err != nil {
		t.Logf("got expected failure for %s: %v", name, err)
	} else {
		t.Fatalf("%s should have failed but got pitch %d", name, p)
	}
}

func TestFailures(t *testing.T) {
	pitchMustFail(t, "b5") // lowercase unacceptable
	pitchMustFail(t, "G#10") // pitch 128 - outside midi range
	pitchMustFail(t, "A-1") // negative octave
	pitchMustFail(t, "G♯#4") // double accidental unacceptable
	pitchMustFail(t, "F") // missing octave
	pitchMustFail(t, "F#")
	pitchMustFail(t, "")
	pitchMustFail(t, "♭A5")
}
