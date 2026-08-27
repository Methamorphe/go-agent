package diag

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"
)

type PprofServer struct {
	server   *http.Server
	listener net.Listener
	logger   *slog.Logger
}

func StartPprof(
	address string,
	logger *slog.Logger,
) (*PprofServer, error) {
	if strings.TrimSpace(address) == "" {
		return nil, nil
	}

	if err := validateLoopbackAddress(
		address,
	); err != nil {
		return nil, err
	}

	listener, err := net.Listen(
		"tcp",
		address,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"listen pprof: %w",
			err,
		)
	}

	mux := http.NewServeMux()

	mux.HandleFunc(
		"/debug/pprof/",
		pprof.Index,
	)

	mux.HandleFunc(
		"/debug/pprof/cmdline",
		pprof.Cmdline,
	)

	mux.HandleFunc(
		"/debug/pprof/profile",
		pprof.Profile,
	)

	mux.HandleFunc(
		"/debug/pprof/symbol",
		pprof.Symbol,
	)

	mux.HandleFunc(
		"/debug/pprof/trace",
		pprof.Trace,
	)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	result := &PprofServer{
		server:   server,
		listener: listener,
		logger:   logger,
	}

	go func() {
		err := server.Serve(listener)

		if err != nil &&
			err != http.ErrServerClosed {
			logger.Error(
				"pprof server stopped",
				"error",
				err,
			)
		}
	}()

	logger.Info(
		"pprof enabled",
		"address",
		listener.Addr().String(),
	)

	return result, nil
}

func (s *PprofServer) Close(
	ctx context.Context,
) error {
	if s == nil {
		return nil
	}

	return s.server.Shutdown(ctx)
}

func validateLoopbackAddress(
	address string,
) error {
	host, _, err :=
		net.SplitHostPort(address)

	if err != nil {
		return fmt.Errorf(
			"invalid pprof address: %w",
			err,
		)
	}

	if strings.EqualFold(
		host,
		"localhost",
	) {
		return nil
	}

	ip := net.ParseIP(host)

	if ip == nil ||
		!ip.IsLoopback() {
		return fmt.Errorf(
			"pprof must bind to an explicit loopback address",
		)
	}

	return nil
}
