package helper

import "io"

// Bridge copies data bidirectionally between a and b until either side closes.
// Both sides are closed before returning. Returns the first non-nil error
// encountered during copying, or nil.
//
// The io.ReadWriteCloser signature lets callers pass net.Conn directly or
// supply an aggregate type that combines a buffered reader, a synchronized
// writer, and a separate closer.
func Bridge(a, b io.ReadWriteCloser) error {
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
