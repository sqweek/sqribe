package main

import (
	"github.com/sqweek/fluidsynth"
)

func SynthInit(srate int, sfont string) (*fluidsynth.Synth, error) {
	settings := make(map[string]interface{})
	settings["audio.period-size"] = srate
	settings["audio.sample-format"] = "16bits"
	settings["synth.gain"] = 0.6
	synth := fluidsynth.NewSynth(settings)
	synth.SFLoad(sfont, true)
	return synth, nil
}
