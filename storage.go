package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"database/sql"
	_ "github.com/mattn/go-sqlite3"

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
		a, dberr := filesDB.AudioFile(file)
		if dberr != nil {
			fmt.Printf("db error: retrieving linked audio file: %s: %v\n", file, dberr)
		}
		files.Audio = a
		if files.Audio == "" {
			files.Audio = s.Headers().String("Filename")
		}
	} else {
		files.Audio = file
		states, dberr := filesDB.StateFiles(file)
		if dberr != nil {
			fmt.Printf("db error: retrieving linked state file: %s: %v\n", file, dberr)
		}
		if len(states) == 0 {
			files.State = fs.SaveDir() + "/" + stateKey(file)
		} else {
			files.State = states[0]
			if len(states) > 1 {
				// XXX should allow user to choose which state to load
				fmt.Printf("multiple states available for audio %s; using first\n", file)
				for _, f := range states {
					fmt.Printf("	%s\n", f)
				}
			}
		}
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

func SaveState(files FileContext, s State) (err error) {
	var tmpfile *os.File
	d, f := filepath.Split(files.State)
	if tmpfile, err = ioutil.TempFile(d, f); err == nil {
		err = s.Write(tmpfile)
		tmpfile.Close()
		if err == nil {
			if err = fs.ReplaceFile(tmpfile.Name(), files.State); err == nil {
				if dberr := filesDB.Associate(files.State, files.Audio); dberr != nil {
					fmt.Printf("db error: associating %s -> %s: %v\n", files.State, files.Audio, dberr)
				}
			}
		} else {
			os.Remove(tmpfile.Name())
		}
	}
	return
}

type AssociationConflict struct {
	Statefile string
	Attempt string // the audio file we're attempting to link to
	Existing string // the audio file already linked in the DB
}

func (e *AssociationConflict) Error() string {
	return fmt.Sprintf("Can't associate %s with %s - it's already associated with %s", e.Statefile, e.Attempt, e.Existing)
}

type FilesDB interface {
	StateFiles(audiofile string) ([]string, error)
	AudioFile(statefile string) (string, error)
	Associate(statefile, audiofile string) error
}

type filesSqlite struct {
	db *sql.DB
	initialised bool
}

var filesDB filesSqlite

func (f *filesSqlite) withDB(fn func(db *sql.DB)error) (err error) {
	if f.db == nil {
		f.db, err = sql.Open("sqlite3", fs.SaveDir() + "/files.qps?_busy_timeout=3500")
		if err != nil {
			return err
		}
	}
	if !f.initialised {
		if err = f.createSchema(f.db); err != nil {
			return err
		}
		f.initialised = true
	}
	return fn(f.db)
}

func (f *filesSqlite) createSchema(db *sql.DB) (err error) {
	var tx *sql.Tx
	if tx, err = db.Begin(); err == nil {
		var vers int
		defer commitUnlessErr(tx, &err)
		row := tx.QueryRow("PRAGMA schema_version;")
		if err = row.Scan(&vers); err == nil && vers == 0 {
			_, err = tx.Exec("CREATE TABLE paths (state TEXT NOT NULL PRIMARY KEY, audio TEXT NOT NULL, CHECK(length(state) > 0 AND length(audio) > 0));")
		}
	}
	return
}

func (f *filesSqlite) StateFiles(audiofile string) (statefiles []string, err error) {
	err = f.withDB(func(db *sql.DB) (err error) {
		var rows *sql.Rows
		if rows, err = db.Query("SELECT state FROM paths WHERE audio = ?", audiofile); err == nil {
			defer rows.Close()
			for rows.Next() {
				var s string
				rows.Scan(&s)
				statefiles = append(statefiles, s)
			}
		}
		return
	})
	return
}

func (f *filesSqlite) AudioFile(statefile string) (audiofile string, err error) {
	err = f.withDB(func(db *sql.DB) (err error) {
		row := db.QueryRow("SELECT audio FROM paths WHERE state = ?", statefile)
		return row.Scan(&audiofile)
	})
	return
}

func (f *filesSqlite) Associate(statefile, audiofile string) error {
	return f.withDB(func (db *sql.DB) (err error) {
		var tx *sql.Tx
		if tx, err = db.Begin(); err != nil {
			return
		}
		defer commitUnlessErr(tx, &err)
		row := tx.QueryRow("SELECT audio FROM paths WHERE state = ?", statefile)
		var audio string
		if err = row.Scan(&audio); err == sql.ErrNoRows {
			_, err = tx.Exec("INSERT INTO paths VALUES (?, ?)", statefile, audiofile)
		} else if err == nil && audio != audiofile {
			err = &AssociationConflict{statefile, audiofile, audio}
		}
		return
	})
}

func commitUnlessErr(tx *sql.Tx, err *error) {
	if *err == nil {
		*err = tx.Commit()
	}
	if *err != nil {
		tx.Rollback()
	}
}
