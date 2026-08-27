//go:build windows

package control

import (
	"context"
	"net"

	winio "github.com/Microsoft/go-winio"
)

func dial(
	ctx context.Context,
	address string,
) (net.Conn, error) {
	return winio.DialPipeContext(
		ctx,
		address,
	)
}
