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
uint32_t prototype_tls_bump(void);
uint32_t prototype_tls_constructor_passed(void);
uint32_t prototype_retire_init(void);
void prototype_retire_mark(void);
uint32_t prototype_retired(void);
uint32_t prototype_thread_tls(void);
*/
import "C"

import (
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"cgo-prototype/fpcell"
)

// Keep the main goroutine on the initial thread throughout, so that tests
// can rely on other goroutines running on Go-created pthreads.
func init() {
	runtime.LockOSThread()
}

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
	// The initial thread's C TLS is initialized before C constructors run.
	if C.prototype_tls_constructor_passed() == 0 {
		panic("C constructor saw uninitialized thread-local storage")
	}

	// C TLS is per thread: the calling thread's copy keeps counting, while a
	// new pthread sees a fresh copy of the initialized image.
	runtime.LockOSThread()
	first := uint32(C.prototype_tls_bump())
	if first < 124 {
		panic(first)
	}
	if got := uint32(C.prototype_tls_bump()); got != first+1 {
		panic(got)
	}
	runtime.UnlockOSThread()
	if got := uint32(C.prototype_thread_tls()); got != 0 {
		panic(got)
	}
	println("cgo prototype: C thread-local storage passed")

	// A goroutine that exits while locked retires its thread, which must
	// leave through pthread_exit (running TSD destructors) and let later Ms
	// reuse the retired threads' resources. Keep the main goroutine on the
	// initial thread (see init) so that every retiring goroutine runs on a
	// Go-created pthread.
	if C.prototype_retire_init() == 0 {
		panic("pthread_key_create failed")
	}
	for i := 0; i < 4; i++ {
		done := make(chan uint32)
		go func() {
			runtime.LockOSThread()
			C.prototype_retire_mark()
			done <- uint32(C.prototype_add32(C.uint32_t(i), 1))
		}()
		if got := <-done; got != uint32(i)+1 {
			panic(got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for deadline := time.Now().Add(10 * time.Second); C.prototype_retired() < 4; {
		if time.Now().After(deadline) {
			panic("retired Go threads did not exit through pthread_exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
	locked := make(chan uint32)
	for i := 0; i < 4; i++ {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			locked <- uint32(C.prototype_thread_call_go(C.uint32_t(200 + i)))
		}()
	}
	for i := 0; i < 4; i++ {
		if got := <-locked; got < 202 || got > 205 {
			panic(got)
		}
	}
	println("cgo prototype: retired locked threads passed")

	// Two cgo packages taking the address of the same C function each have
	// their own cell, and both hold the same table index.
	if free := unsafe.Pointer(C.free); free == nil || free != fpcell.Free() {
		panic("C.free function pointers differ across packages")
	}
	println("cgo prototype: cross-package C function pointers passed")

	// The runtime mirrors os.Setenv into the C environment.
	name := C.CString("CGO_PROTOTYPE_ENV")
	if err := os.Setenv("CGO_PROTOTYPE_ENV", "value"); err != nil {
		panic(err)
	}
	if got := C.GoString(C.getenv(name)); got != "value" {
		panic("C getenv after os.Setenv = " + got)
	}
	if err := os.Unsetenv("CGO_PROTOTYPE_ENV"); err != nil {
		panic(err)
	}
	if C.getenv(name) != nil {
		panic("C getenv after os.Unsetenv is set")
	}
	C.free(unsafe.Pointer(name))
	println("cgo prototype: C environment passed")

	// With cgo, syscall's credential calls go through libc so that every
	// thread changes together. Values and errno must cross the boundary.
	if err := syscall.Setgroups([]int{0}); err != nil {
		panic(err)
	}
	if err := syscall.Setgid(0); err != nil {
		panic(err)
	}
	if err := syscall.Setresuid(0, 0, 0); err != nil {
		panic(err)
	}
	if err := syscall.Setuid(-1); err != syscall.EINVAL {
		panic(err)
	}
	println("cgo prototype: libc credential calls passed")

	println("cgo prototype: C-created pthread callbacks passed")
}
