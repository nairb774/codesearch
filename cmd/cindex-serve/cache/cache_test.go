package cache

import (
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRacingLoad(t *testing.T) {
	maxProc := runtime.GOMAXPROCS(0)
	var counter uint64
	c := New(context.Background(), uint64(maxProc),
		func(_ context.Context, key string) (int, error) {
			if atomic.AddUint64(&counter, 1)&0xFF == 0 {
				return 0, io.EOF
			}
			return len(key), nil
		},
		func(_ context.Context, key string, value []byte) error {
			copy(value, []byte(key))
			if atomic.AddUint64(&counter, 1)&0xFF == 0 {
				return io.EOF
			}
			return nil
		})

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 2*maxProc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for time.Since(start) < time.Second {
				for i := 0; i < 256; i++ {
					k := string([]byte{byte(i)})
					func() {
						v := c.Acquire(k)
						defer c.Release(v)
						got, err := v.Get(context.Background())
						if err == ErrMemoryPressure {
							return
						}
						if err == io.EOF {
							if got != nil {
								t.Errorf("Got (%v, %v), want (nil, EOF)", got, err)
							}
						} else if err != nil || string(got) != k {
							t.Errorf("Got (%v, %v), want (%v, nil)", got, err, k)
						}
					}()
				}
				func() {
					c.mu.Lock()
					defer c.mu.Unlock()
					if v := c.available + c.inLRU + c.inUse; v != uint64(maxProc) {
						t.Errorf("Bad book keeping: got: %v want: %v. available: %v inLRU: %v inUse: %v", v, maxProc, c.available, c.inLRU, c.inUse)
					}
					if uint64(c.lru.Len()) != c.inLRU {
						t.Errorf("Bad LRU accounting. Got %v, want %v", c.lru.Len(), c.inLRU)
					}
					if uint64(len(c.items)) != c.inLRU+c.inUse {
						t.Errorf("Bad map accounting. Got %v, want %v", len(c.items), c.inLRU+c.inUse)
					}
				}()
			}
		}()
	}

	wg.Wait()
}

func TestAcquireRelease(t *testing.T) {
	c := New(context.Background(), 2,
		func(_ context.Context, key string) (int, error) {
			return len(key), nil
		},
		func(_ context.Context, key string, value []byte) error {
			copy(value, []byte(key))
			return nil
		})
	for _, k := range []string{
		"a",
		"b",
		"c",
		"d",
		"e",
		"f",
		"g",
	} {
		t.Run(k, func(t *testing.T) {
			v := c.Acquire(k)
			defer c.Release(v)
			got, err := v.Get(context.Background())
			if err != nil || string(got) != k {
				t.Errorf("Got (%v, %v), want (%v, nil)", got, err, k)
			}
			if len(c.items) > 2 {
				t.Errorf("%v", c.items)
			}
		})
	}
}

func TestReleaseWhileLoading(t *testing.T) {
	c := New(context.Background(), 1,
		func(context.Context, string) (int, error) { return 1, nil },
		func(ctx context.Context, _ string, _ []byte) error {
			<-ctx.Done()
			return ctx.Err()
		})

	func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		e := c.Acquire("")
		defer c.Release(e)
		e.Get(ctx)

		f := c.Acquire("")
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
}

func TestAlwaysLoadError(t *testing.T) {
	myErr := errors.New("myErr")
	c := New(context.Background(), 1,
		func(context.Context, string) (int, error) { return 1, nil },
		func(context.Context, string, []byte) error { return myErr })

	func() {
		e := c.Acquire("")
		defer c.Release(e)
		v, err := e.Get(context.Background())
		if v != nil || err != myErr {
			t.Errorf("Unexpected return: %v %v", v, err)
		}
		if c.available != 1 {
			t.Errorf("Invalid token state: %#v", c)
		}
	}()
}
