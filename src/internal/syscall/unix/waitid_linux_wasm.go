// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unix

import (
	"syscall"
	"unsafe"
)

const (
	P_PID   = 1
	P_PIDFD = 3
)

// kernelRusage is the wasm32 kernel's struct rusage, with 32-bit timevals and
// longs. syscall.Rusage has musl's time64 layout.
type kernelRusage struct {
	Utime  [2]int32
	Stime  [2]int32
	Fields [14]int32
}

func Waitid(idType int, id int, info *SiginfoChild, options int, rusage *syscall.Rusage) error {
	var k *kernelRusage
	if rusage != nil {
		k = new(kernelRusage)
	}
	_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, uintptr(idType), uintptr(id), uintptr(unsafe.Pointer(info)), uintptr(options), uintptr(unsafe.Pointer(k)), 0)
	if errno != 0 {
		return errno
	}
	if rusage != nil {
		*rusage = syscall.Rusage{
			Utime:    syscall.Timeval{Sec: int64(k.Utime[0]), Usec: int64(k.Utime[1])},
			Stime:    syscall.Timeval{Sec: int64(k.Stime[0]), Usec: int64(k.Stime[1])},
			Maxrss:   k.Fields[0],
			Ixrss:    k.Fields[1],
			Idrss:    k.Fields[2],
			Isrss:    k.Fields[3],
			Minflt:   k.Fields[4],
			Majflt:   k.Fields[5],
			Nswap:    k.Fields[6],
			Inblock:  k.Fields[7],
			Oublock:  k.Fields[8],
			Msgsnd:   k.Fields[9],
			Msgrcv:   k.Fields[10],
			Nsignals: k.Fields[11],
			Nvcsw:    k.Fields[12],
			Nivcsw:   k.Fields[13],
		}
	}
	return nil
}
