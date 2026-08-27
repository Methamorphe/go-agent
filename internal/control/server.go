package control

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/Methamorphe/go-agent/internal/control/protocol"
)

type Server struct {
	logger   *slog.Logger
	handler  Handler
	maxFrame int

	sem chan struct{}

	mu    sync.Mutex
	conns map[net.Conn]struct{}
	wg    sync.WaitGroup
}

func NewServer(
	logger *slog.Logger,
	handler Handler,
	maxFrameBytes int,
	maxConnections int,
) *Server {
	return &Server{
		logger:   logger,
		handler:  handler,
		maxFrame: maxFrameBytes,

		sem: make(
			chan struct{},
			maxConnections,
		),

		conns: make(
			map[net.Conn]struct{},
		),
	}
}

func (s *Server) Serve(
	ctx context.Context,
	listener net.Listener,
) error {
	shutdownDone := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
			s.closeConnections()

		case <-shutdownDone:
		}
	}()

	defer close(shutdownDone)

	for {
		conn, err := listener.Accept()

		if err != nil {
			if ctx.Err() != nil ||
				errors.Is(
					err,
					net.ErrClosed,
				) {
				break
			}

			return fmt.Errorf(
				"control accept: %w",
				err,
			)
		}

		select {
		case s.sem <- struct{}{}:
			s.track(conn)

			s.wg.Add(1)

			go s.serveConn(
				ctx,
				conn,
			)

		default:
			s.logger.Warn(
				"control connection rejected",
				"reason",
				"connection limit reached",
			)

			_ = conn.Close()
		}
	}

	s.wg.Wait()

	return nil
}

func (s *Server) serveConn(
	ctx context.Context,
	conn net.Conn,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error(
				"panic isolated at control connection boundary",
				"panic",
				recovered,
			)
		}

		s.untrack(conn)

		_ = conn.Close()

		<-s.sem

		s.wg.Done()
	}()

	decoder := protocol.NewDecoder(
		conn,
		s.maxFrame,
	)

	encoder := protocol.NewEncoder(
		conn,
		s.maxFrame,
	)

	for {
		request, err := decoder.Decode()

		if err != nil {
			if errors.Is(err, io.EOF) ||
				errors.Is(err, net.ErrClosed) ||
				ctx.Err() != nil {
				return
			}

			s.logger.Warn(
				"invalid control frame",
				"error",
				err,
			)

			return
		}

		response, err :=
			s.handler.Handle(
				ctx,
				request,
			)

		if err != nil {
			response =
				protocol.ErrorResponse(
					request,
					err,
				)
		}

		if err := encoder.Encode(
			response,
		); err != nil {
			s.logger.Warn(
				"control response write failed",
				"error",
				err,
			)

			return
		}
	}
}

func (s *Server) track(
	conn net.Conn,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.conns[conn] = struct{}{}
}

func (s *Server) untrack(
	conn net.Conn,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.conns, conn)
}

func (s *Server) closeConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.conns {
		_ = conn.Close()
	}
}
