// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !wasm

package syscall

import "unsafe"

func rawSyscallNoError(trap, a1, a2, a3 uintptr) (r1, r2 uintptr)
func rawVforkSyscall(trap, a1, a2, a3 uintptr) (r1 uintptr, err Errno)

func wasmForkAndExec(argv0 *byte, argv, envv []*byte, chroot, dir *byte, attr *ProcAttr, sys *SysProcAttr, pipe int) (int, Errno) {
	return 0, ENOSYS
}

func wasmExecve(path *byte, argv, envv []*byte) error {
	return ENOSYS
}

func setsockoptSockFprog(fd int, program *SockFprog) error {
	return setsockopt(fd, SOL_SOCKET, SO_ATTACH_FILTER, unsafe.Pointer(program), unsafe.Sizeof(*program))
}
