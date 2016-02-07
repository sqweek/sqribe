package main

import (
	"fmt"
	"github.com/sqweek/fs"

	"sqweek.net/sqribe/log"
)

var Usr *fs.Dirs
var App *fs.Dirs

func fsinit(fqdn, name string) error {
	var err error
	if Usr, err = fs.UserDirs(); err != nil {
		return err
	}
	if App, err = fs.AppDirs(fqdn, name); err != nil {
		return err
	}
	return nil
}

func MustFind(filename string) string {
	f, err := App.Locate(filename)
	if err != nil {
		log.FS.Printf("%#v", App)
		fatal(fmt.Sprintf("error: required file %s not found in search path", filename))
	}
	return f
}
