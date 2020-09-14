package cache

import (
	"container/list"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/google/codesearch/index2"
)

var (
	ErrMemoryPressure = errors.New("not enough free meemory")
)

type CacheLoader interface {
	Size(ctx context.Context, name string) (length int32, err error)
	Load(ctx context.Context, name string, offset int64, body []byte) error
}

func New(ctx context.Context, available int64, loader CacheLoader) *Cache {
	return &Cache{
		workCtx:   ctx,
		loader:    loader,
		items:     make(map[index.SHA256]Entry),
		available: available,
	}
}

type Cache struct {
	workCtx context.Context
	loader  CacheLoader

	mu    sync.Mutex
	items map[index.SHA256]Entry
	lru   list.List // *string (keys) - use * to avoid allocation.

	available int64 // Can immediately allocate (may be negative).
	inLRU     int64 // count=0
	inUse     int64 // count>0
}

type LoadParams struct {
	Start  int64
	Length int32

	// If true, then resource limits are ignored, and loads are alwyas allowed.
	IgnoreLimit bool
}

func (c *Cache) Acquire(key index.SHA256, name string, params LoadParams) Entry {
	c.mu.Lock()
	defer c.mu.Unlock()

	e := c.innerAcquire(key)
	if e == nil {
		e = &prepare{
			cache:  c,
			key:    key,
			name:   name,
			params: params,
		}
	}
	return e
}

func (c *Cache) innerAcquire(key index.SHA256) Entry {
	e := c.items[key]
	switch e := e.(type) {
	case *loading:
		e.entry.acquire(c)

	case *entry:
		e.acquire(c)
	}

	return e
}

func (c *Cache) tryLoad(key index.SHA256, name string, params LoadParams) (Entry, error) {
	if params.Start < 0 || params.Length < 0 {
		return nil, fmt.Errorf("bad params values: %+v", params)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if e := c.innerAcquire(key); e != nil {
		return e, nil
	}

	tokens := int64(params.Length)
	if !params.IgnoreLimit && c.available+c.inLRU < tokens {
		return nil, ErrMemoryPressure
	}

	for c.available < tokens && c.lru.Len() > 0 {
		key := *(c.lru.Remove(c.lru.Back()).(*index.SHA256))
		tokens := c.items[key].(*entry).allocatedSize()
		c.available += tokens
		c.inLRU -= tokens
		delete(c.items, key)
	}

	c.available -= tokens
	c.inUse += tokens

	ctx, cancel := context.WithCancel(c.workCtx)
	l := &loading{
		cancel: cancel,
		done:   make(chan struct{}),
		entry: &entry{
			key:   key,
			body:  make([]byte, int(params.Length)),
			count: 1,
		},
	}

	go func() {
		defer cancel()
		l.err = c.loader.Load(ctx, name, params.Start, l.entry.body)
		if l.err == nil {
			if got := sha256.Sum256(l.entry.body); got != key {
				l.err = fmt.Errorf("key %064x with %v got %064x", key, params, got)
			}
		}
		c.finish(l)
	}()

	c.items[key] = l

	return l, nil
}

func (c *Cache) Release(e Entry) {
	switch e := e.(type) {
	case *prepare:
		c.Release(e.entry)

	case *loading:
		c.mu.Lock()
		if e.entry.count--; e.entry.count != 0 {
			c.mu.Unlock()
			return
		}

		select {
		case <-e.done:
			if e.err == nil {
				e.entry.element = c.lru.PushFront(&e.entry.key)
				tokens := e.entry.allocatedSize()
				c.inLRU += tokens
				c.inUse -= tokens
			}
			c.mu.Unlock()

		default:
			e.cancel()
			delete(c.items, e.entry.key)
			c.mu.Unlock()
			<-e.done
		}

	case *entry:
		c.mu.Lock()
		defer c.mu.Unlock()
		if e.count--; e.count == 0 {
			e.element = c.lru.PushFront(&e.key)
			tokens := e.allocatedSize()
			c.inLRU += tokens
			c.inUse -= tokens
		}
	}
}

func (c *Cache) finish(l *loading) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if l.err != nil {
		delete(c.items, l.entry.key)
		tokens := l.entry.allocatedSize()
		c.inUse -= tokens
		c.available += tokens
		l.entry.body = nil
	} else if l.entry.count > 0 {
		c.items[l.entry.key] = l.entry
	}

	close(l.done)
}

type Entry interface {
	Get(context.Context) ([]byte, error)
}

type prepare struct {
	cache  *Cache
	key    index.SHA256
	name   string
	params LoadParams

	entry Entry
}

func (p *prepare) Get(ctx context.Context) (data []byte, err error) {
	if p.entry == nil {
		if p.params.Length == 0 {
			p.params.Length, err = p.cache.loader.Size(ctx, p.name)
			if err != nil {
				return nil, err
			}
		}
		p.entry, err = p.cache.tryLoad(p.key, p.name, p.params)
		if err != nil {
			return nil, err
		}
	}
	return p.entry.Get(ctx)
}

type loading struct {
	// Allows canceling loading - useful when `count` drops to 0.
	cancel context.CancelFunc

	// done is closed when `entry` has been set to the loaded value, or there was
	// an error during loading.
	done chan struct{}

	// entry is the entry being loaded.
	entry *entry
	err   error
}

func (l *loading) Get(ctx context.Context) ([]byte, error) {
	select {
	case <-l.done:
		if l.err != nil {
			return nil, l.err
		}
		return l.entry.Get(ctx)

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type entry struct {
	key  index.SHA256
	body []byte

	// References the current location in the LRU. Will only be set if `count` is
	// zero.
	element *list.Element

	// count is the number of outstanding "locks" held on the entry. Must only be
	// changed when the cache's `mu` is held. If zero, `element` will be non-nil.
	// If >0 then `element` will be nil.
	count int
}

func (e *entry) acquire(c *Cache) {
	if e.count++; e.count == 1 {
		tokens := e.allocatedSize()
		c.inLRU -= tokens
		c.inUse += tokens
		c.lru.Remove(e.element)
		e.element = nil
	}
}

func (e *entry) Get(context.Context) ([]byte, error) { return e.body, nil }
func (e *entry) allocatedSize() int64                { return int64(len(e.body)) }
