//go:build windows

package control

import (
	"fmt"
	"net"

	winio "github.com/Microsoft/go-winio"
)

func Listen(
	address string,
) (net.Listener, error) {
	listener, err :=
		winio.ListenPipe(
			address,
			nil,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"listen on named pipe: %w",
			err,
		)
	}

	return listener, nil
}
