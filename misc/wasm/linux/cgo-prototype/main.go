// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	uint32_t a;
	uint64_t b;
} prototype_pair;

typedef struct {
	uint32_t *data;
	uint32_t length;
} prototype_buffer;

typedef struct {
	uint32_t *items[2];
} prototype_pointer_table;

typedef uint32_t (*prototype_transform)(uint32_t);

uint32_t prototype_add32(uint32_t a, uint32_t b);
uint64_t prototype_mix64(uint64_t a, uint64_t b);
uint32_t prototype_bump(uint32_t *value);
uint32_t prototype_checksum(const uint8_t *data, uint32_t length);
uint64_t prototype_pair_sum(prototype_pair pair);
uint32_t prototype_buffer_sum(prototype_buffer buffer);
prototype_buffer prototype_make_buffer(uint32_t *data, uint32_t length);
uint32_t prototype_pointer_table_sum(prototype_pointer_table table);
uint32_t prototype_increment(uint32_t value);
prototype_transform prototype_get_transform(void);
uint32_t prototype_apply(prototype_transform transform, uint32_t value);
uint32_t prototype_set_errno(uint32_t value);
uint32_t prototype_call_go(uint32_t value);
uint32_t prototype_call_go_pointer(uint32_t value);
uint32_t prototype_call_go_pointer_return(uint32_t value);
uint32_t prototype_call_go_buffer(uint32_t *data, uint32_t length);
uint32_t prototype_thread_call_go(uint32_t value);
*/
import "C"

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

//go:noinline
func callbackStack(depth int, value uint32) uint32 {
	var scratch [128]byte
	scratch[depth%len(scratch)] = byte(value)
	if depth == 0 {
		return value + uint32(scratch[0]&0)
	}
	return callbackStack(depth-1, value) + uint32(scratch[depth%len(scratch)]&0)
}

//export GoCallback
func GoCallback(value C.uint32_t) C.uint32_t {
	if uint32(value)&7 == 0 {
		runtime.Gosched()
	}
	return C.uint32_t(callbackStack(32, uint32(value)) + 1)
}

//export GoPointerCallback
func GoPointerCallback(value *C.uint32_t) C.uint32_t {
	*value = *value + 1
	return *value
}

//export GoPointerReturn
func GoPointerReturn(value *C.uint32_t) *C.uint32_t {
	return value
}

//export GoBufferCallback
func GoBufferCallback(buffer C.prototype_buffer) C.prototype_buffer {
	return buffer
}

func main() {
	println("cgo prototype: Go main entered")
	if got := uint32(C.prototype_add32(20, 22)); got != 42 {
		panic(got)
	}
	println("cgo prototype: C call passed")
	if got := uint64(C.prototype_mix64(0x1122334455667788, 0x0102030405060708)); got != 0x1020304050607080 {
		panic(got)
	}

	value := C.uint32_t(41)
	if got := uint32(C.prototype_bump(&value)); got != 42 || value != 42 {
		panic(got)
	}
	native := C.malloc(C.size_t(unsafe.Sizeof(C.uint32_t(0))))
	if native == nil {
		panic("malloc")
	}
	*(*C.uint32_t)(native) = 41
	if got := uint32(C.prototype_bump((*C.uint32_t)(native))); got != 42 {
		panic(got)
	}
	C.free(native)

	bytes := []byte{1, 2, 3, 4, 5, 6}
	buf := C.CBytes(bytes)
	if buf == nil {
		panic("CBytes")
	}
	if got := uint32(C.prototype_checksum((*C.uint8_t)(buf), C.uint32_t(len(bytes)))); got != 21 {
		panic(got)
	}
	if copied := C.GoBytes(buf, C.int(len(bytes))); len(copied) != len(bytes) || copied[5] != 6 {
		panic("GoBytes")
	}
	C.free(buf)

	pair := C.prototype_pair{a: 7, b: 0x100000002}
	if got := uint64(C.prototype_pair_sum(pair)); got != 0x100000009 {
		panic(got)
	}
	values := []C.uint32_t{3, 5, 7}
	buffer := C.prototype_buffer{data: &values[0], length: C.uint32_t(len(values))}
	if got := uint32(C.prototype_buffer_sum(buffer)); got != 15 {
		panic(got)
	}
	returned := C.prototype_make_buffer(&values[0], C.uint32_t(len(values)))
	if returned.data != &values[0] || returned.length != C.uint32_t(len(values)) {
		panic("aggregate return")
	}
	table := C.prototype_pointer_table{items: [2]*C.uint32_t{&values[0], &values[2]}}
	if got := uint32(C.prototype_pointer_table_sum(table)); got != 10 {
		panic(got)
	}
	transform := C.prototype_get_transform()
	if transform == nil || uint32(C.prototype_apply(transform, 41)) != 42 {
		panic("function pointer")
	}
	if got := uint32(C.prototype_apply((*[0]byte)(C.prototype_increment), 41)); got != 42 {
		panic("named C function pointer")
	}
	if got, err := C.prototype_set_errno(123); uint32(got) != 7 || err != syscall.Errno(123) {
		panic(err)
	}
	println("cgo prototype: pointer, aggregate, and errno ABI passed")

	if got := uint32(C.prototype_call_go(40)); got != 42 {
		panic(got)
	}
	if got := uint32(C.prototype_call_go_pointer(40)); got != 42 {
		panic(got)
	}
	if got := uint32(C.prototype_call_go_pointer_return(41)); got != 42 {
		panic(got)
	}
	if got := uint32(C.prototype_call_go_buffer(&values[0], C.uint32_t(len(values)))); got != 15 {
		panic(got)
	}
	println("cgo prototype: C-to-Go callback passed")

	runtime.GOMAXPROCS(2)
	const workers = 4
	const rounds = 25
	var wg sync.WaitGroup
	errs := make(chan uint32, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for round := 0; round < rounds; round++ {
				value := uint32(worker*rounds + round)
				if got := uint32(C.prototype_call_go(C.uint32_t(value))); got != value+2 {
					errs <- got
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	close(errs)
	for got := range errs {
		panic(got)
	}
	runtime.GC()
	println("cgo prototype: concurrent scheduled callbacks passed")

	for value := uint32(104); value < 108; value++ {
		if got := uint32(C.prototype_thread_call_go(C.uint32_t(value))); got != value+2 {
			panic(got)
		}
	}
	println("cgo prototype: C-created pthread callbacks passed")
}
