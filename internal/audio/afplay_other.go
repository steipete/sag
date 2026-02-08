//go:build !darwin

package audio

import (
	"context"
	"errors"
	"io"
)

// StreamViaAfplay is not available on non-macOS platforms.
func StreamViaAfplay(_ context.Context, _ io.Reader) error {
	return errors.New("afplay backend is only available on macOS")
}
