package bounded

import (
	"fmt"
	"sync"
)

type Preview struct {
	mu        sync.Mutex
	max       int
	headLimit int
	tailLimit int
	head      []byte
	tail      []byte
	total     int64
}

func NewPreview(max int) *Preview {
	if max < 2 {
		max = 2
	}
	head := max / 2
	return &Preview{
		max:       max,
		headLimit: head,
		tailLimit: max - head,
		head:      make([]byte, 0, head),
		tail:      make([]byte, 0, max-head),
	}
}

func (p *Preview) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	original := len(data)
	p.total += int64(original)

	if len(p.head) < p.headLimit {
		need := p.headLimit - len(p.head)
		if need > len(data) {
			need = len(data)
		}
		p.head = append(p.head, data[:need]...)
		data = data[need:]
	}

	if len(data) > 0 {
		p.tail = append(p.tail, data...)
		if len(p.tail) > p.tailLimit {
			over := len(p.tail) - p.tailLimit
			copy(p.tail, p.tail[over:])
			p.tail = p.tail[:p.tailLimit]
		}
	}
	return original, nil
}

func (p *Preview) Total() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.total
}

func (p *Preview) Truncated() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.total > int64(p.max)
}

func (p *Preview) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.total <= int64(p.max) {
		result := make([]byte, 0, len(p.head)+len(p.tail))
		result = append(result, p.head...)
		result = append(result, p.tail...)
		return string(result)
	}

	return fmt.Sprintf(
		"%s\n\n... <%d bytes omitted> ...\n\n%s",
		string(p.head),
		p.total-int64(len(p.head))-int64(len(p.tail)),
		string(p.tail),
	)
}
