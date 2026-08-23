# Static cgo integration test

This directory is the executable `linux/wasm` static-cgo integration test. It
uses the distro's `wasm32-unknown-linux-musl` Clang/sysroot and exercises the
complete internal-link path:

- musl crt1 owns startup, TLS, the process break, and `memory.grow`;
- cmd/link ingests the relocatable WebAssembly selected by the C driver,
  including native signatures, code/data/table/TLS relocations, constructors,
  libc, pthread, and compiler-runtime archive members;
- Go-to-C calls switch to `m.g0`, and C-to-Go callbacks can schedule, grow and
  copy the goroutine stack, and return through the live C activation;
- callbacks from C-created pthreads borrow and release a Go M while using the
  new WebAssembly instance's private globals;
- the strict indirect-call ABI consistently uses `int32(void*)`, returning
  zero for ordinary wrappers and errno for two-result cgo calls;
- Go pointers remain 64-bit while C pointers remain memory32. Generated call
  and export wrappers check narrowing, zero-extend results, and marshal
  pointers nested in aggregate values field by field;
- named C functions selected in Go are carried as native table indices rather
  than Go continuation PCs; and
- libc allocation, direct pointers, `C.CBytes`/`C.GoBytes`, scalar and
  aggregate parameters/results, function pointers, errno, scheduled callbacks,
  and pthread-created callbacks all execute in one program.

The emitted module is validated with the threads and exception features and
has passed the distro VM with 1, 2, and 4 virtual CPUs. The test deliberately
forces callback stack growth and scheduler handoffs so a scalar-only success
cannot mask an invalid stack-switch implementation.

The build remains static and on Go's internal-link path. Dynamic libraries,
plugins, PIE, the race detector, fork, and asynchronous preemption are outside
the target platform contract. C `longjmp` or an exception must not cross a Go
frame.

A pointer to an unknown-length array of C structs whose layout changes at the
32/64-bit boundary cannot be repacked automatically. Libraries with that API
shape need an explicit count/accessor wrapper or compact `uintptr_t` transport
fields. Aggregate values and fixed-size arrays remain generated translations.

See [DESIGN.md](DESIGN.md) for the fixed ownership/ABI decisions and remaining
validation work.
