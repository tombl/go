// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

type cgoWasmAllocArgs struct {
	size   uint32
	align  uint32
	result uint32
}

//go:nosplit
func cgoWasmAlloc(n uintptr) unsafe.Pointer {
	if _cgo_wasm_alloc == nil || uint64(n) > uint64(^uint32(0)) {
		return nil
	}
	args := cgoWasmAllocArgs{size: uint32(n), align: uint32(physPageSize)}
	asmcgocall(_cgo_wasm_alloc, unsafe.Pointer(&args))
	return unsafe.Pointer(uintptr(args.result))
}

func sbrk(n uintptr) unsafe.Pointer {
	n = memRound(n)
	if iscgo {
		// libc owns the process break in a cgo program. The sbrk allocator
		// remains Go's suballocator, but each new backing region comes from
		// musl instead of directly executing memory.grow.
		return cgoWasmAlloc(n)
	}
	bl := bloc
	if bl+n > blocMax {
		grow := (bl + n - blocMax) / physPageSize
		size := growMemory(int32(grow))
		if size < 0 {
			return nil
		}
		resetMemoryDataView()
		blocMax = bl + n
	}
	bloc += n
	return unsafe.Pointer(bl)
}

// Implemented in src/runtime/sys_wasm.s
func growMemory(pages int32) int32
func currentMemory() int32
