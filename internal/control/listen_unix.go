//go:build !windows

package control

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

type unixListener struct {
	net.Listener
	path string
}

func Listen(
	address string,
) (net.Listener, error) {
	if err := os.MkdirAll(
		filepath.Dir(address),
		0o700,
	); err != nil {
		return nil, fmt.Errorf(
			"create control socket directory: %w",
			err,
		)
	}

	if info, err :=
		os.Lstat(address); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf(
				"control address exists and is not a socket: %s",
				address,
			)
		}

		if err := os.Remove(address); err != nil {
			return nil, fmt.Errorf(
				"remove stale control socket: %w",
				err,
			)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf(
			"inspect control socket: %w",
			err,
		)
	}

	listener, err := net.Listen(
		"unix",
		address,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"listen on unix socket: %w",
			err,
		)
	}

	if err := os.Chmod(
		address,
		0o600,
	); err != nil {
		_ = listener.Close()
		_ = os.Remove(address)

		return nil, fmt.Errorf(
			"chmod control socket: %w",
			err,
		)
	}

	return &unixListener{
		Listener: listener,
		path:     address,
	}, nil
}

func (l *unixListener) Close() error {
	err := l.Listener.Close()

	_ = os.Remove(l.path)

	return err
}
