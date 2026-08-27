//go:build !windows

package control

import (
	"context"
	"net"
)

func dial(
	ctx context.Context,
	address string,
) (net.Conn, error) {
	var dialer net.Dialer

	return dialer.DialContext(
		ctx,
		"unix",
		address,
	)
}
