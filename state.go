package main

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"fmt"
	"os"
)

type state interface {
	Capture() // captures current memory model state
	Restore() // restores this objects state to the memory model
}

type stateV1 struct {
	Filename string
	Beats []FrameN
}

func (s *stateV1) Capture() {
	s.Filename = G.audiofile
	s.Beats = G.score.beats
}

func (s *stateV1) Restore() {
	G.score.beats = s.Beats
}

type VersionHeader struct {
	Version int
}

var currentVersion = VersionHeader{1}

func stateV(hdr VersionHeader) state {
	switch (hdr.Version) {
	case 1:
		return &stateV1{}
	}
	panic(fmt.Sprintf("unknown version %d", hdr.Version))
}

func flatpath(r rune) rune {
	if r < 26 || strings.ContainsRune(" /", r) {
		return '_'
	}
	return r
}

func key(filename string) string {
	return strings.TrimLeft(strings.Map(flatpath, filename) + ".sqs", "_")
}

func LoadState(filename string) error {
	stateFile := SaveDir() + "/" + key(filename)
	if _, err := os.Stat(stateFile); err == nil {
		f, err := os.Open(stateFile)
		if err != nil {
			return err
		}
		defer f.Close()
		j := json.NewDecoder(f)
		var version VersionHeader
		err = j.Decode(&version)
		if err != nil {
			return err
		}
		s := stateV(version)
		err = j.Decode(&s)
		if err != nil {
			return err
		}
		s.Restore()
	}
	return nil
}

func SaveState(filename string) error {
	k := key(filename)
	tmpfile, err := ioutil.TempFile(SaveDir(), k)
	if err != nil {
		return err
	}
	defer tmpfile.Close()
	j := json.NewEncoder(tmpfile)
	j.Encode(&currentVersion)
	s := stateV(currentVersion)
	s.Capture()
	err = j.Encode(s)
	if err != nil {
		return err
	}
	err = os.Rename(tmpfile.Name(), SaveDir() + "/" + k)
	if err != nil {
		return err
	}
	return nil
}