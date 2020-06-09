package cache

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/codesearch/index2"
)

var keys = map[index.SHA256]string{}

func init() {
	for i := 0; i < 256; i++ {
		k := []byte{byte(i)}
		keys[sha256.Sum256(k)] = string(k)
	}
}

type racingLoader uint64

func (r *racingLoader) Size(_ context.Context, name string) (uint32, error) {
	if atomic.AddUint64((*uint64)(r), 1)&0xFF == 0 {
		return 0, io.EOF
	}
	return uint32(len(name)), nil
}

func (r *racingLoader) Load(_ context.Context, name string, _ uint64, value []byte) error {
	copy(value, []byte(name))
	if atomic.AddUint64((*uint64)(r), 1)&0xFF == 0 {
		return io.EOF
	}
	return nil
}

func TestRacingLoad(t *testing.T) {
	maxProc := runtime.GOMAXPROCS(0)
	var loader racingLoader
	c := New(context.Background(), int64(maxProc), &loader)

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 2*maxProc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for time.Since(start) < time.Second {
				for k, v := range keys {
					k, v := k, v
					func() {
						e := c.Acquire(k, v, LoadParams{})
						defer c.Release(e)
						got, err := e.Get(context.Background())
						if err == ErrMemoryPressure {
							return
						}
						if err == io.EOF {
							if got != nil {
								t.Errorf("Got (%v, %v), want (nil, EOF)", got, err)
							}
						} else if err != nil || string(got) != v {
							t.Errorf("Got (%v, %v), want (%v, nil)", got, err, v)
						}
					}()
				}
				func() {
					c.mu.Lock()
					defer c.mu.Unlock()
					if v := c.available + c.inLRU + c.inUse; v != int64(maxProc) {
						t.Errorf("Bad book keeping: got: %v want: %v. available: %v inLRU: %v inUse: %v", v, maxProc, c.available, c.inLRU, c.inUse)
					}
					if int64(c.lru.Len()) != c.inLRU {
						t.Errorf("Bad LRU accounting. Got %v, want %v", c.lru.Len(), c.inLRU)
					}
					if int64(len(c.items)) != c.inLRU+c.inUse {
						t.Errorf("Bad map accounting. Got %v, want %v", len(c.items), c.inLRU+c.inUse)
					}
				}()
			}
		}()
	}

	wg.Wait()
}

type simpleLoader struct{}

func (simpleLoader) Size(_ context.Context, name string) (uint32, error) {
	return uint32(len(name)), nil
}

func (simpleLoader) Load(_ context.Context, name string, _ uint64, value []byte) error {
	copy(value, []byte(name))
	return nil
}

func TestAcquireRelease(t *testing.T) {
	c := New(context.Background(), 2, simpleLoader{})
	for k, v := range keys {
		t.Run(v, func(t *testing.T) {
			e := c.Acquire(k, v, LoadParams{})
			defer c.Release(e)
			got, err := e.Get(context.Background())
			if err != nil || string(got) != v {
				t.Errorf("Got (%v, %v), want (%v, nil)", got, err, v)
			}
			if len(c.items) > 2 {
				t.Errorf("%v", c.items)
			}
		})
	}
}

type slowLoad struct{}

func (slowLoad) Size(context.Context, string) (uint32, error) {
	return 1, nil
}
func (slowLoad) Load(ctx context.Context, _ string, _ uint64, _ []byte) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestReleaseWhileLoading(t *testing.T) {
	c := New(context.Background(), 1, slowLoad{})

	for k, v := range keys {
		t.Run(v, func(t *testing.T) {
			func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				e := c.Acquire(k, v, LoadParams{})
				defer c.Release(e)
				e.Get(ctx)

				f := c.Acquire(k, v, LoadParams{})
				defer c.Release(f)
				f.Get(ctx)
			}()
			if a := c.available; a != 1 {
				t.Errorf("%v", a)
			}
			if len(c.items) != 0 {
				t.Errorf("Crap left behind: %v", c.items)
			}
			if c.available != 1 {
				t.Errorf("Invalid token state: %#v", c)
			}
		})
	}
}

type fixedError struct {
	error
}

func (fixedError) Size(context.Context, string) (uint32, error) {
	return 1, nil
}
func (e fixedError) Load(context.Context, string, uint64, []byte) error {
	return e.error
}

func TestAlwaysLoadError(t *testing.T) {
	myErr := errors.New("myErr")
	c := New(context.Background(), 1, fixedError{myErr})

	for k, v := range keys {
		t.Run(v, func(t *testing.T) {
			e := c.Acquire(k, v, LoadParams{})
			defer c.Release(e)
			v, err := e.Get(context.Background())
			if v != nil || err != myErr {
				t.Errorf("Unexpected return: %v %v", v, err)
			}
			if c.available != 1 {
				t.Errorf("Invalid token state: %#v", c)
			}
		})
	}
}
