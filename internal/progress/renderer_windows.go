//go:build windows

package progress

import "os"

func isTerminal(_ *os.File) bool {
	return false
}
