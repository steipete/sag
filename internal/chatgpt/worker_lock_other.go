//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package chatgpt

import (
	"errors"
	"os"
)

const supportsWarm = false

func lockWorker(string) (*os.File, error) {
	return nil, errors.New("ChatGPT voice keep-alive requires Unix")
}
