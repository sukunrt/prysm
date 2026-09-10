package client

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestMultiLock_ConcurrentReadersAndIndependentKeys(t *testing.T) {
	l := newMultilock()
	l.RLock("readers")
	defer l.RUnlock("readers")
	l.Lock("writer")
	defer l.Unlock("writer")

	readDone := make(chan struct{})
	go func() {
		l.RLock("readers")
		l.RUnlock("readers")
		close(readDone)
	}()
	writeDone := make(chan struct{})
	go func() {
		l.Lock("another writer")
		l.Unlock("another writer")
		close(writeDone)
	}()

	for _, done := range []<-chan struct{}{readDone, writeDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("independent lock acquisition blocked")
		}
	}
}

func TestMultiLock_ConcurrentReadWrite(t *testing.T) {
	for _, test := range []struct {
		name string
		lock *multiLock
	}{
		{name: "zero value", lock: &multiLock{}},
		{name: "constructor", lock: newMultilock()},
	} {
		t.Run(test.name, func(t *testing.T) {
			const workers, iterations = 8, 100
			var first, second int
			var wg sync.WaitGroup
			start := make(chan struct{})
			for range workers {
				wg.Go(func() {
					<-start
					for range iterations {
						test.lock.Lock("shared")
						first++
						runtime.Gosched()
						second++
						test.lock.Unlock("shared")

						test.lock.RLock("shared")
						a, b := first, second
						test.lock.RUnlock("shared")
						if a != b {
							t.Errorf("reader observed a partial write: %d != %d", a, b)
						}
					}
				})
			}
			close(start)
			wg.Wait()
			require.Equal(t, workers*iterations, first)
			require.Equal(t, first, second)
		})
	}
}
