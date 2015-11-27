package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sqweek.net/sqribe/fs"
)

type FileContext struct {
	Audio string
	State string
	Timestamp time.Time // last modified time when state was read. 0 => fresh state (file didn't exist)
}

// file can be either a .sqs file or audio file
func Open(file string) (files FileContext, s State, err error) {
	if IsStateFilename(file) {
		files.State = file
		if s, files.Timestamp, err = LoadState(file); err != nil {
			return
		}
		files.Audio = s.Headers().String("Filename")
	} else {
		files.Audio = file
		files.State = StateFile(file)
		if s, files.Timestamp, err = LoadState(files.State); err != nil {
			return
		}
	}
	return
}

func IsStateFilename(file string) bool {
	return strings.HasSuffix(strings.ToLower(file), ".sqs")
}

func flatpath(r rune) rune {
	if r < 26 || strings.ContainsRune(" /:\\", r) {
		return '_'
	}
	return r
}

func stateKey(audiofile string) string {
	return strings.TrimLeft(strings.Map(flatpath, audiofile) + ".sqs", "_")
}

func StateFile(audiofile string) string {
	return fs.SaveDir() + "/" + stateKey(audiofile)
}

func LoadState(statefile string) (s State, modTime time.Time, err error) {
	var f *os.File
	if f, err = os.Open(statefile); err != nil {
		if os.IsNotExist(err) {
			return EmptyState(), modTime, nil
		}
		return nil, modTime, err
	}
	defer f.Close()
	var stat os.FileInfo
	if stat, err = f.Stat(); err != nil {
		return nil, modTime, err
	}
	if s, err = ReadState(f); err != nil {
		return nil, modTime, err
	}
	return s, stat.ModTime(), nil
}

func SaveState(statefile string, s State) (err error) {
	var tmpfile *os.File
	d, f := filepath.Split(statefile)
	if tmpfile, err = ioutil.TempFile(d, f); err == nil {
		err = s.Write(tmpfile)
		tmpfile.Close()
		if err == nil {
			err = fs.ReplaceFile(tmpfile.Name(), statefile)
		} else {
			os.Remove(tmpfile.Name())
		}
	}
	return
}


