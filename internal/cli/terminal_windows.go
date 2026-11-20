package cli

import (
	"os"
	"syscall"
)

// isTerminal reports whether f is a console. Windows gives every character
// device, NUL included, the same file ID, so os.SameFile cannot tell them apart.
func isTerminal(f *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil
}
