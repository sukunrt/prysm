package client

import "sync"

// multiLock retains per-key read/write locks for its lifetime; its zero value is ready to use.
type multiLock struct {
	mx    sync.RWMutex
	locks map[string]*sync.RWMutex
}

func newMultilock() *multiLock {
	return &multiLock{
		locks: map[string]*sync.RWMutex{},
	}
}

func (l *multiLock) RLock(key string) {
	l.lockFor(key).RLock()
}

func (l *multiLock) RUnlock(key string) {
	l.lockFor(key).RUnlock()
}

func (l *multiLock) Lock(key string) {
	l.lockFor(key).Lock()
}

func (l *multiLock) Unlock(key string) {
	l.lockFor(key).Unlock()
}

func (l *multiLock) lockFor(key string) *sync.RWMutex {
	l.mx.RLock()
	if kl, ok := l.locks[key]; ok {
		l.mx.RUnlock()
		return kl
	}
	l.mx.RUnlock()
	l.mx.Lock()
	defer l.mx.Unlock()
	if kl, ok := l.locks[key]; ok {
		return kl
	}
	if l.locks == nil {
		l.locks = make(map[string]*sync.RWMutex)
	}
	kl := &sync.RWMutex{}
	l.locks[key] = kl
	return kl
}
