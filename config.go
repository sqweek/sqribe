package main

import (
	"encoding/json"
	"github.com/sqweek/fs"
	"os"
	"time"

	"github.com/sqweek/sqribe/log"
)

type ConfigJSON struct {
	FS struct {
		SaveDir string
		SoundFont string
	}
	UI struct {
		Scale int
	}
}

var Cfg struct {
	ConfigJSON

	mtime time.Time // mtime of the config file when it was loaded
}

func confinit() {
	Cfg.FS.SaveDir = App.Docs
	mtime, p, err := ReadConfig(fs.SingleConfigPath(Usr, App, "sqribe.json"))
	if err == nil {
		applyConfig(mtime, &p)
	} else if !os.IsNotExist(err) {
		log.FS.Println("loading config failed:", err)
	}
}

func ReadConfig(path string) (mtime time.Time, p ConfigJSON, err error) {
	var f *os.File
	var st os.FileInfo
	if st, err = os.Stat(path); err == nil {
		mtime = st.ModTime()
		if f, err = os.Open(path); err == nil {
			defer f.Close()
			j := json.NewDecoder(f)
			err = j.Decode(&p)
		}
	}
	return
}

// Applies a parsed config to the memory model
func applyConfig(mtime time.Time, params *ConfigJSON) {
	if params.FS.SaveDir != "" {
		Cfg.FS.SaveDir = params.FS.SaveDir
	}
	if params.FS.SoundFont != "" {
		Cfg.FS.SoundFont = params.FS.SoundFont
	}
	if params.UI.Scale > 0 {
		Cfg.UI.Scale = params.UI.Scale
		yspacing = 2 * Cfg.UI.Scale
	}
	Cfg.mtime = mtime
}
