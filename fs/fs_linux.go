package fs

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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

func ReplaceFile(src, dst string) error {
	return os.Rename(src, dst)
}

func ExeDir() string {
	var path string
	if strings.Contains(os.Args[0], "/") {
		path = os.Args[0]
	} else {
		p, err := exec.LookPath(os.Args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "couldn't find self in path:", err)
			return "."
		}
		path = p
	}
	// absolute or relative path; just drop the executable name
	dirs := strings.Split(path, "/")
	return strings.Join(dirs[:len(dirs)-1], "/")
}
