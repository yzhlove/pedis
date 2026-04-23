package helper

import (
	"io"
	"net"
)

// Bridge copies data bidirectionally between a and b until either side closes.
// Both connections are closed before returning. Returns the first non-nil error
// encountered during copying, or nil.
func Bridge(a, b net.Conn) error {
	defer a.Close()
	defer b.Close()

	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(a, b)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(b, a)
		errCh <- err
	}()
	return <-errCh
}
