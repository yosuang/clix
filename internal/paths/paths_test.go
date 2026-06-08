package paths

import (
	"path/filepath"
	"testing"
)

func TestLayoutFromHomeUsesUserGlobalPaths(t *testing.T) {
	// #given
	home := filepath.Join("home", "alice")

	// #when
	got := FromHome(home)

	// #then
	if got.ToolsDir != filepath.Join(home, ".config", "clix", "tools") {
		t.Fatalf("ToolsDir = %q", got.ToolsDir)
	}
	if got.DatabasePath != filepath.Join(home, ".local", "share", "clix", "clix.db") {
		t.Fatalf("DatabasePath = %q", got.DatabasePath)
	}
}
