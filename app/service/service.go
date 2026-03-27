package service

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
)

type Service interface {
	Init() error
	Start() error
	Stop() error
}

func Run(s ...Service) error {
	// 1. Init all services sequentially.
	for _, svc := range s {
		if err := svc.Init(); err != nil {
			return err
		}
	}

	errCh := make(chan error, len(s))

	// 2. Start all services concurrently.
	for _, svc := range s {
		sv := svc
		go func(sv Service) {
			errCh <- sv.Start()
		}(sv)
	}

	// 3. Listen for OS signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		// A service failed — stop everything and return the original error.
		stopErr := stopAll(s)
		return errors.Join(err, stopErr)

	case <-sigCh:
		// Signal received — stop everything and collect errors.
		return stopAll(s)
	}
}

// stopAll calls Stop on every service and joins any errors.
func stopAll(services []Service) error {
	errs := make([]error, 0, len(services))
	for _, svc := range services {
		if err := svc.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
