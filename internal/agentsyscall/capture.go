package agentsyscall

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/Methamorphe/go-agent/internal/bounded"
	"github.com/Methamorphe/go-agent/internal/objectstore"
)

var errOutputLimit = errors.New("output limit exceeded")

type captureResult struct {
	meta objectstore.Meta
	err  error
}

type capture struct {
	mu       sync.Mutex
	writer   *io.PipeWriter
	reader   *io.PipeReader
	preview  *bounded.Preview
	max      int64
	total    int64
	exceeded bool
	cancel   context.CancelFunc
	result   chan captureResult
	closed   bool
}

func newCapture(objects ObjectStore, previewBytes int, maxBytes int64, cancel context.CancelFunc) *capture {
	reader, writer := io.Pipe()
	capture := &capture{
		writer:  writer,
		reader:  reader,
		preview: bounded.NewPreview(previewBytes),
		max:     maxBytes,
		cancel:  cancel,
		result:  make(chan captureResult, 1),
	}
	persistCtx := context.WithoutCancel(context.Background())
	go func() {
		meta, err := objects.Put(persistCtx, reader)
		_ = reader.CloseWithError(err)
		capture.result <- captureResult{meta: meta, err: err}
	}()
	return capture
}

func (c *capture) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, io.ErrClosedPipe
	}
	if c.max > 0 && c.total+int64(len(data)) > c.max {
		remaining := c.max - c.total
		if remaining > 0 {
			part := data[:remaining]
			_, _ = c.preview.Write(part)
			written, err := c.writer.Write(part)
			c.total += int64(written)
			if err != nil {
				return written, err
			}
		}
		c.exceeded = true
		if c.cancel != nil {
			c.cancel()
		}
		return int(remaining), errOutputLimit
	}

	_, _ = c.preview.Write(data)
	written, err := c.writer.Write(data)
	c.total += int64(written)
	return written, err
}

func (c *capture) Close() (objectstore.Meta, error) {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		_ = c.writer.Close()
	}
	c.mu.Unlock()

	result := <-c.result
	_ = c.reader.Close()
	return result.meta, result.err
}

func (c *capture) Preview() string { return c.preview.String() }
func (c *capture) Truncated() bool { return c.preview.Truncated() }

func (c *capture) Exceeded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exceeded
}
