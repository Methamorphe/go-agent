package control

import (
	"context"
	"fmt"
)

// Probe verifies that the local runtime control endpoint accepts connections.
// It deliberately sends no protocol frame: the server treats a clean EOF as a
// normal client disconnect, so this does not mutate canonical runtime state.
func Probe(ctx context.Context, address string) error {
	conn, err := dial(ctx, address)
	if err != nil {
		return fmt.Errorf("connect to runtime: %w", err)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("close runtime probe: %w", err)
	}
	return nil
}
