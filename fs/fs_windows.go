package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	kernel = syscall.MustLoadDLL("kernel32.dll")
	getModuleFileName = kernel.MustFindProc("GetModuleFileNameW")
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
			if err = os.Remove(bak); err == nil {
				continue /* old backup is gone, rename should work now */
			}
		}
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		os.Remove(bak)
		return nil
	} else {
		return err
	}
}

func exeFileName() string {
	buf := make([]uint16, syscall.MAX_PATH)
	r, _, err := getModuleFileName.Call(0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		fmt.Println("error: GetModuleFileNameW: ", err)
		return ""
	}
	return string(utf16.Decode(buf[0:uint32(r)]))
}

func ExeDir() string {
	return filepath.Dir(exeFileName())
}
