package paths

import (
	"os"
	"path/filepath"
)

type Layout struct {
	ToolsDir     string
	DatabasePath string
}

func FromHome(home string) Layout {
	return Layout{
		ToolsDir:     filepath.Join(home, ".config", "clix", "tools"),
		DatabasePath: filepath.Join(home, ".local", "share", "clix", "clix.db"),
	}
}

func Resolve() (Layout, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Layout{}, err
	}
	return FromHome(home), nil
}
