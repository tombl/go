// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package poll

import (
	"runtime"
	"syscall"
	"unsafe"
)

// Go's wasm port uses 64-bit Go pointers, while the Linux/wasm C ABI uses
// 32-bit pointers. Keep syscall.Iovec source-compatible with other Go ports
// and marshal it to the kernel layout at the syscall boundary.
type wasmIovec struct {
	base uint32
	len  uint32
}

func writev(fd int, iovecs []syscall.Iovec) (uintptr, error) {
	wiovecs := make([]wasmIovec, len(iovecs))
	for i := range iovecs {
		wiovecs[i].base = uint32(uintptr(unsafe.Pointer(iovecs[i].Base)))
		wiovecs[i].len = iovecs[i].Len
	}

	for {
		r, _, e := syscall.Syscall(syscall.SYS_WRITEV, uintptr(fd), uintptr(unsafe.Pointer(&wiovecs[0])), uintptr(len(wiovecs)))
		runtime.KeepAlive(iovecs)
		if e == syscall.EINTR {
			continue
		}
		if e != 0 {
			return r, e
		}
		return r, nil
	}
}
