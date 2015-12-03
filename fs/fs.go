package fs

import (
	"os"
	"strings"

	"sqweek.net/sqribe/log"
)

func Join(dirs... string) string {
	return strings.Join(dirs, string(os.PathSeparator))
}

func Find(filename string, paths... string) (string, error) {
	return find(filename, append(paths, ExeDir(), ".")...)
}

func find(filename string, paths... string) (string, error) {
	for _, path := range paths {
		f := Join(path, filename)
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			return f, nil
		}
	}
	return "", os.ErrNotExist
}

func MustFind(filename string, paths... string) string {
	f, err := Find(filename, paths...)
	if err != nil {
		log.FS.Println("error: couldn't find", filename, " - search path:")
		for _, path := range paths {
			log.FS.Println(" *", path)
		}
		log.FS.Println(" *", ExeDir())
		log.FS.Println(" *", ".")
		os.Exit(1)
	}
	return f
}
