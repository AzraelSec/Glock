package utils

import "os"

func DirExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
