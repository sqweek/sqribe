package main

import (
	"fmt"
	"os"
)

var cacheDir string
var saveDir string

func init() {
	cacheDir = os.Getenv("HOME") + "/.cache/sqribe"
	saveDir = os.Getenv("HOME") + "/.local/share/sqribe"
	err := os.MkdirAll(cacheDir, 0755)
	if err != nil {
		panic(err)
	}
	err = os.MkdirAll(saveDir, 0755)
	if err != nil {
		panic(err)
	}
}

func CacheFile() string {
	return fmt.Sprintf("%s/%d", cacheDir, os.Getpid())
}

func SaveDir() string {
	return saveDir
}
