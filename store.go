package main

import (
	"io/ioutil"
	"os"
	"strings"

	"sqweek.net/sqribe/fs"
)

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

func SaveState(audiofile string, s State) (err error) {
	var tmpfile *os.File
	k := stateKey(audiofile)
	if tmpfile, err = ioutil.TempFile(fs.SaveDir(), k); err == nil {
		err = s.Write(tmpfile)
		tmpfile.Close()
		if err == nil {
			err = fs.ReplaceFile(tmpfile.Name(), fs.SaveDir() + "/" + k)
		} else {
			os.Remove(tmpfile.Name())
		}
	}
	return
}


