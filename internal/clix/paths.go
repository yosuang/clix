package clix

import (
	"os"
	"path/filepath"
	"runtime"
)

func defaultManifestPath() (string, *AppError) {
	home, err := userHomeDir()
	if err != nil {
		return "", newError(CodeManifestError, err.Error())
	}
	return filepath.Join(home, ".config", "clix", "manifest.yaml"), nil
}

func defaultDatabasePath() (string, *AppError) {
	home, err := userHomeDir()
	if err != nil {
		return "", newError(CodeStorageError, err.Error())
	}
	return filepath.Join(home, ".local", "share", "clix", "clix.db"), nil
}

func userHomeDir() (string, error) {
	if runtime.GOOS == "windows" {
		if home := os.Getenv("USERPROFILE"); home != "" {
			return home, nil
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	return os.UserHomeDir()
}
