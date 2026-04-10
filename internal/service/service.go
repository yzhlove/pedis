package service

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
)

// Service is the lifecycle interface for all long-running components.
type Service interface {
	Init() error
	Start() error
	Stop() error
}

// Run initialises all services, starts them concurrently, and blocks until
// a signal or one of the services returns an error.
func Run(s ...Service) error {
	for _, svc := range s {
		if err := svc.Init(); err != nil {
			return err
		}
	}

	errCh := make(chan error, len(s))

	for _, svc := range s {
		sv := svc
		go func(sv Service) {
			errCh <- sv.Start()
		}(sv)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		return errors.Join(err, stopAll(s))
	case <-sigCh:
		return stopAll(s)
	}
}

func stopAll(services []Service) error {
	errs := make([]error, 0, len(services))
	for _, svc := range services {
		if err := svc.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
