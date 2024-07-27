package os

import (
	"os"
	"path/filepath"
	"runtime"
)

// homeDir returns the OS-specific home path as specified in the environment.
func HomeDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"))
	}
	return os.Getenv("HOME")
}
