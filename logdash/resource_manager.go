package logdash

import (
	"context"
	"errors"
)

type (
	ResourceManager interface {
		Shutdown(ctx context.Context) error
		Close() error
	}

	noopResourceManager struct{}
)

var ErrAlreadyClosed = errors.New("already closed or shutting down")

func (noopResourceManager) Shutdown(ctx context.Context) error {
	return nil
}

func (noopResourceManager) Close() error {
	return nil
}
