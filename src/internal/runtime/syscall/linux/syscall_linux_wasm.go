// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package linux

//go:wasmimport linux syscall
//go:nosplit
func wasmSyscall(num, a1, a2, a3, a4, a5, a6 uint32) int32

// Syscall6 calls system call number num with arguments a1-6.
//
//go:nosplit
func Syscall6(num, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, errno uintptr) {
	r := wasmSyscall(uint32(num), uint32(a1), uint32(a2), uint32(a3), uint32(a4), uint32(a5), uint32(a6))
	if r < 0 && r >= -4095 {
		return ^uintptr(0), 0, uintptr(-r)
	}
	return uintptr(uint32(r)), 0, 0
}
