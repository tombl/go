// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdint.h>
#include <unistd.h>
#include "libcgo.h"

typedef struct {
	uint32_t size;
	uint32_t align;
	uint32_t result;
} wasm_alloc_args;

// x_cgo_wasm_alloc is the nonallocating ABI bridge used while the Go heap is
// being initialized. libc is the sole owner of memory.grow and the process
// break in cgo programs; Go suballocates the aligned regions returned here.
CGO_ASMCGOCALL_RETURN_TYPE
x_cgo_wasm_alloc(void *opaque)
{
	wasm_alloc_args *args = (wasm_alloc_args *)opaque;
	uintptr_t extra = args->align - 1;
	void *base;
	uintptr_t aligned;
	uint32_t reserve;

	if (!args->align || (args->align & extra) ||
	    args->size > UINT32_MAX - args->align) {
		args->result = 0;
		CGO_ASMCGOCALL_RETURN;
	}
	// musl's wasm sbrk serializes memory.grow and returns the old break in
	// one operation. libc's malloc uses 4 KiB-aligned increments while Go
	// reserves at 64 KiB alignment, so reserve one complete alignment unit
	// of padding and select the aligned interior. Unlike align-1 padding, an
	// align-sized increment preserves libc's page-aligned process break.
	reserve = args->size + args->align;
	base = sbrk((intptr_t)reserve);
	if (base == (void *)-1) {
		args->result = 0;
		CGO_ASMCGOCALL_RETURN;
	}
	aligned = ((uintptr_t)base + extra) & ~extra;
	args->result = (uint32_t)aligned;
	CGO_ASMCGOCALL_RETURN;
}
