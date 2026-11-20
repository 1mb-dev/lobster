//go:build !windows

package cli

import "os"

// isTerminal reports whether f is a character device other than os.DevNull,
// which is one too but has nobody behind it to answer a prompt.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(fi, null)
}
