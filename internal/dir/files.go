package dir

import (
	"fmt"
	"os"
	"path/filepath"
)

func DirExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// FindNearestFile searches upward from currentPath for targetFileName,
// stopping after hop parent traversals.
func FindNearestFile(currentPath string, hop int, targetFileName string) (string, error) {
	candidate := filepath.Join(currentPath, targetFileName)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	parent := filepath.Dir(currentPath)
	if parent == currentPath || hop == 0 {
		return "", fmt.Errorf("no %s file found", targetFileName)
	}

	return FindNearestFile(parent, hop-1, targetFileName)
}
