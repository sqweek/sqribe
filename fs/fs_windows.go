package fs

import (
	"fmt"
	"os"
	"strings"
	"github.com/Allendang/w32"
)

var cacheDir string
var saveDir string

func init() {
	cacheDir = os.Getenv("TEMP") + "\\sqribe"
	saveDir = os.Getenv("APPDATA") + "\\sqribe"
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
	bak := dst + ".bak"
	for {
		err := os.Rename(dst, bak)
		if err == nil || os.IsNotExist(err) {
			/* rename succeeded or 'dst' doesn't exist yet; we can proceed */
			break
		} else if os.IsExist(err) {
			err = os.Remove(bak)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	err := os.Rename(src, dst)
	if err == nil {
		os.Remove(bak)
		return nil
	}
	return err
}

func ExeDir() string {
	path := w32.GetModuleFileName(nil)
	dirs := strings.Split(path, "\\")
	return strings.Join(dirs[:len(dirs)-1], "\\")
}
