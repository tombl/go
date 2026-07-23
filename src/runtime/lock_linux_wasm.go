// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && wasm

package runtime

import "internal/runtime/atomic"

const (
	mutexUnlocked = 0
	mutexLocked   = 1
	mutexSleeping = 2

	active_spin     = 4
	active_spin_cnt = 30

	mutexMLocksDelta = 16
)

type mWaitList struct{}

func lockVerifyMSize() {}

//go:nosplit
func mutexContended(l *mutex) bool {
	return atomic.Load(key32(&l.key)) != mutexUnlocked
}

func lock(l *mutex) {
	lockWithRank(l, getLockRank(l))
}

//go:nosplit
func lock2(l *mutex) {
	gp := getg()
	if gp.m.locks < 0 {
		throw("lock count")
	}
	gp.m.locks += mutexMLocksDelta

	key := key32(&l.key)
	if atomic.Cas(key, mutexUnlocked, mutexLocked) {
		return
	}
	for {
		if atomic.Xchg(key, mutexSleeping) == mutexUnlocked {
			return
		}
		futexsleep(key, mutexSleeping, -1)
	}
}

func unlock(l *mutex) {
	unlockWithRank(l)
}

//go:nosplit
func unlock2(l *mutex) {
	gp := getg()
	gp.m.locks -= mutexMLocksDelta
	if gp.m.locks < 0 {
		throw("lock count")
	}

	key := key32(&l.key)
	state := atomic.Xchg(key, mutexUnlocked)
	if state == mutexUnlocked {
		throw("unlock of unlocked lock")
	}
	if state == mutexSleeping {
		futexwakeup(key, 1)
	}
}
