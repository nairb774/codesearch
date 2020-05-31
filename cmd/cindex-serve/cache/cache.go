package cache

import (
	"container/list"
	"context"
	"errors"
	"sync"
)

var (
	ErrMemoryPressure = errors.New("not enough free meemory")
)

func New(ctx context.Context, available uint64, size func(context.Context, string) (int, error), load func(context.Context, string, []byte) error) *Cache {
	return &Cache{
		workCtx:   ctx,
		size:      size,
		load:      load,
		items:     make(map[string]Entry),
		available: available,
	}
}

type Cache struct {
	workCtx context.Context
	size    func(context.Context, string) (int, error)
	load    func(context.Context, string, []byte) error

	mu    sync.Mutex
	items map[string]Entry
	lru   list.List // *string (keys) - use * to avoid allocation.

	available uint64 // Can immediately allocate
	inLRU     uint64 // count=0
	inUse     uint64 // count>0
}

func (c *Cache) Acquire(key string) Entry {
	c.mu.Lock()
	defer c.mu.Unlock()

	e := c.innerAcquire(key)
	if e == nil {
		e = &prepare{
			cache: c,
			key:   key,
		}
	}
	return e
}

func (c *Cache) innerAcquire(key string) Entry {
	e := c.items[key]
	switch e := e.(type) {
	case *loading:
		e.entry.acquire(c)

	case *entry:
		e.acquire(c)
	}

	return e
}

func (c *Cache) tryLoad(key string, size int) (Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e := c.innerAcquire(key); e != nil {
		return e, nil
	}

	tokens := uint64(size)
	if c.available+c.inLRU < tokens {
		return nil, ErrMemoryPressure
	}

	for c.available < tokens {
		key := *(c.lru.Remove(c.lru.Back()).(*string))
		tokens := c.items[key].(*entry).allocatedSize()
		c.available += uint64(tokens)
		c.inLRU -= uint64(tokens)
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
			body:  make([]byte, size),
			count: 1,
		},
	}

	go func() {
		defer cancel()
		l.err = c.load(ctx, key, l.entry.body)
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
	cache *Cache
	key   string

	entry Entry
}

func (p *prepare) Get(ctx context.Context) ([]byte, error) {
	size, err := p.cache.size(ctx, p.key)
	if err != nil {
		return nil, err
	}
	p.entry, err = p.cache.tryLoad(p.key, size)
	if err != nil {
		return nil, err
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
	key  string
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
func (e *entry) allocatedSize() uint64               { return uint64(len(e.body)) }
