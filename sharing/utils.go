// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"net/url"
	"strings"
	"sync"
	"time"
)

// queryFlagLookup finds a boolean value in a url.Values struct, returning
// defValue if not found.
//  * no flag is the default value
//  * ?flag is assumed true
//  * ?flag=no (or false or 0 or off) is assumed false (case insensitive)
//  * everything else is true
func queryFlagLookup(q url.Values, name string, defValue bool) bool {
	vals, ok := q[name]
	if !ok || len(vals) == 0 {
		// the flag wasn't specified
		return defValue
	}
	val := vals[0]
	if len(val) == 0 {
		// the flag was specified, but no value was provided. must be form of
		// ?flag or ?flag= but no value. assume that means on.
		return true
	}
	switch strings.ToLower(val) {
	case "no", "false", "0", "off":
		// cases where the flag is false
		return false
	}
	return true
}

// MutexGroup is a group of mutexes by name that attempts to only keep track of
// live mutexes. The zero value is okay to use.
type MutexGroup struct {
	mu      sync.Mutex
	names   map[string]*sync.Mutex
	waiters map[string]int
}

func (m *MutexGroup) init(name string) {
	if m.names == nil {
		m.names = map[string]*sync.Mutex{name: new(sync.Mutex)}
		m.waiters = map[string]int{}
		return
	}
	if _, exists := m.names[name]; !exists {
		m.names[name] = &sync.Mutex{}
	}
}

// Lock will lock the mutex named by name. It will return the appropriate
// function to call to unlock that lock.
func (m *MutexGroup) Lock(name string) (unlock func()) {
	m.mu.Lock()
	m.init(name)
	namedMu := m.names[name]
	m.waiters[name]++
	m.mu.Unlock()
	namedMu.Lock()
	return func() {
		namedMu.Unlock()
		m.mu.Lock()
		waiting := m.waiters[name] - 1
		m.waiters[name] = waiting
		if waiting <= 0 {
			if waiting < 0 {
				panic("double unlock")
			}
			delete(m.names, name)
			delete(m.waiters, name)
		}
		m.mu.Unlock()
	}
}

// ExponentialBackoff keeps track of how long we should sleep between
// failing attempts.
type ExponentialBackoff struct {
	delay time.Duration
	Max   time.Duration
	Min   time.Duration
}

func (e *ExponentialBackoff) init() {
	if e.Max == 0 {
		// maximum delay - pulled from net/http.Server.Serve
		e.Max = time.Second
	}
	if e.Min == 0 {
		// minimum delay - pulled from net/http.Server.Serve
		e.Min = 5 * time.Millisecond
	}
}

// Wait should be called when there is a failure. Each time it is called
// it will sleep an exponentially longer time, up to a max.
func (e *ExponentialBackoff) Wait() {
	e.init()
	if e.delay == 0 {
		e.delay = e.Min
	} else {
		e.delay *= 2
	}
	if e.delay > e.Max {
		e.delay = e.Max
	}
	time.Sleep(e.delay)
}

// Maxed returns true if the wait time has maxed out.
func (e *ExponentialBackoff) Maxed() bool {
	e.init()
	return e.delay == e.Max
}
