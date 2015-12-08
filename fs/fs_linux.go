package fs

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func CacheDir() string {
	return os.Getenv("HOME") + "/.cache/sqribe"
}

func SaveDir() string {
	return os.Getenv("HOME") + "/.local/share/sqribe"
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
