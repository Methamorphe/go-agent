package mmu

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"unicode/utf8"
)

type RequestShape struct {
	MessageCount    int
	ToolCount       int
	ToolSchemaBytes int
}

type TokenEstimator interface {
	EstimateText(model, text string) int
	EstimateRequestOverhead(model string, shape RequestShape) int
}

type ConservativeEstimator struct{}

func (ConservativeEstimator) EstimateText(_ string, text string) int {
	if text == "" {
		return 0
	}

	// Deliberately conservative for mixed prose/code without requiring a
	// provider tokenizer. Typical English is closer to ~4 chars/token.
	runes := utf8.RuneCountInString(text)
	return 1 + (runes+2)/3
}

func (ConservativeEstimator) EstimateRequestOverhead(_ string, shape RequestShape) int {
	toolTokens := (shape.ToolSchemaBytes + 2) / 3
	return 8 + shape.MessageCount*6 + shape.ToolCount*12 + toolTokens
}

type tokenCacheEntry struct {
	key    string
	tokens int
}

type cachedEstimator struct {
	base TokenEstimator
	max  int

	mu    sync.Mutex
	items map[string]*list.Element
	lru   *list.List
}

func newCachedEstimator(base TokenEstimator, maxEntries int) *cachedEstimator {
	if base == nil {
		base = ConservativeEstimator{}
	}
	if maxEntries <= 0 {
		maxEntries = 1
	}
	return &cachedEstimator{
		base:  base,
		max:   maxEntries,
		items: make(map[string]*list.Element, maxEntries),
		lru:   list.New(),
	}
}

func (c *cachedEstimator) EstimateText(model, text string) int {
	key := tokenCacheKey(model, text)

	c.mu.Lock()
	if element, ok := c.items[key]; ok {
		c.lru.MoveToFront(element)
		tokens := element.Value.(tokenCacheEntry).tokens
		c.mu.Unlock()
		return tokens
	}
	c.mu.Unlock()

	tokens := c.base.EstimateText(model, text)

	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.items[key]; ok {
		c.lru.MoveToFront(element)
		return element.Value.(tokenCacheEntry).tokens
	}

	element := c.lru.PushFront(tokenCacheEntry{key: key, tokens: tokens})
	c.items[key] = element
	for c.lru.Len() > c.max {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(tokenCacheEntry)
		delete(c.items, entry.key)
		c.lru.Remove(oldest)
	}
	return tokens
}

func (c *cachedEstimator) EstimateRequestOverhead(model string, shape RequestShape) int {
	return c.base.EstimateRequestOverhead(model, shape)
}

func (c *cachedEstimator) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

func tokenCacheKey(model, text string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + text))
	return hex.EncodeToString(sum[:])
}
