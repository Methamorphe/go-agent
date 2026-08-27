package provider

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/Methamorphe/go-agent/internal/errs"
)

const maxSSEEventBytes = 2 << 20

func readSSE(ctx context.Context, src io.Reader, handle func([]byte) error) error {
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 64<<10), maxSSEEventBytes)

	var data bytes.Buffer

	flush := func() error {
		if data.Len() == 0 {
			return nil
		}
		body := append([]byte(nil), data.Bytes()...)
		data.Reset()
		return handle(body)
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}

		part := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data.Len() != 0 {
			if data.Len()+1 > maxSSEEventBytes {
				return errs.New(errs.CodeResourceExhausted, "provider.sse", "SSE event exceeds limit")
			}
			data.WriteByte('\n')
		}
		if data.Len()+len(part) > maxSSEEventBytes {
			return errs.New(errs.CodeResourceExhausted, "provider.sse", "SSE event exceeds limit")
		}
		data.WriteString(part)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := scanner.Err(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "provider.sse", "read stream", err)
	}
	return flush()
}
