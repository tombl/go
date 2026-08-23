# linux/wasm static cgo design checkpoint

This file records the contract validated against the distro toolchain and VM.
It separates decisions that are now fixed from implementation still required
in the Go compiler, runtime, and linker.

## Ownership and startup

musl owns the process break and is the only component allowed to execute
`memory.grow` in a cgo process. Go must not call libc `sbrk` while retaining its
current sbrk allocator: that allocator assumes every byte between two break
values belongs to Go, which becomes false as soon as libc allocates between
them.

Instead, the cgo runtime requests whole aligned regions through a nonallocating
native libc bridge (`aligned_alloc` or an equivalent internal entry point).
The Go heap manages objects inside those regions; libc remains the sole
top-level allocator and serializes growth across instances. A non-cgo binary
keeps the existing direct `memory.grow` path.

The target musl crt1 remains the owner of `_start`. It loads argv through the
kernel ABI, initializes libc and main-thread TLS, runs constructors, and exits
after the program entry returns. The Go executable supplies a native
`__main_argc_argv_envp` adapter which initializes Go and transfers control to
the Go entry point. Go must not duplicate this C startup sequence.

Early Go allocation therefore occurs after libc initialization, but before a
normal goroutine context necessarily exists. The allocator call needs a
no-allocation, no-write-barrier native-call primitive that preserves the wasm
stack global and works on the initial/system stack.

## Threads and TLS

Every pthread is a Web Worker with a fresh `WebAssembly.Instance` and shared
linear memory. Instance globals are private. This matches the two runtimes:

- Go keeps `g` and its continuation registers in private mutable globals.
- LLVM C uses a separate private mutable `__tls_base` global.
- musl's pthread entry sets `__stack_pointer` and `__tls_base`, then invokes
  `__wasm_init_tls` before user code.

For cgo builds, `runtime.newm1` already selects `_cgo_thread_start`; this must
remain the only path that creates Go Ms. It reaches musl `pthread_create`, so C
TLS is initialized before the new instance calls into Go. Raw Go `clone`
remains valid only for `CGO_ENABLED=0`.

The globals coexist; Go TLS does not need to be reimplemented using musl TLS.
The linker lays out non-empty `.tdata`/`.tbss`, synthesizes `__tls_size` and
`__tls_align`, and resolves TLS-relative relocations. Each pthread instance
copies the template through the synthesized `__wasm_init_tls` entry.

## Pointer boundary

The Go ABI remains pointer-width 64 and the C ABI remains wasm32 pointer-width
32. Generated cgo adapters zero-extend C pointers on entry to Go and narrow Go
pointers on entry to C.

All genuine C pointers necessarily fit because they address a memory32 linear
memory capped at 4 GiB. Nevertheless, Go-to-C narrowing must reject nonzero
high bits. `unsafe.Pointer` and integer conversions can manufacture an invalid
64-bit value; silently truncating it would alias an unrelated low address.
The check is an ABI safety invariant, not an allocator fast-path check.

Aggregate field translation stays in generated C/Go wrappers. Pointer-bearing
struct and array values use a natural 64-bit-pointer Go layout and are
marshalled field by field to the compact C layout. A bare pointer to an array
of mixed-layout aggregates is different: C supplies no length from which a
wrapper could infer and repack the pointee range. Such an API needs an explicit
count/accessor bridge, or a compact transport structure containing `uintptr_t`
values. This is an inherent API-boundary requirement rather than a linker or
kernel defect.

Function-pointer typedefs retain their C spelling at the narrowing boundary.
Taking a named C function in a Go expression is implemented through a native
32-bit function-pointer data cell with a table-index relocation; it must not
use the Go function address, which is the distinct continuation value
`PC_F<<16`. WebAssembly's strict function types mean that no generic variadic
or untyped trampoline can substitute for these generated adapters.

All functions passed to `runtime.asmcgocall` use the exact WebAssembly type
`int32(void*)`. Ordinary cgo and runtime/cgo wrappers return zero; errno
wrappers return the captured value. This makes explicit the return-register
convention that native machine ABIs normally leave implicit.

## Static link split

The target C driver performs one whole-program `-r` link after all cgo package
objects and cgo link flags are known. It owns archive member selection, weak
symbol rules, crt1, musl, pthread, and compiler-runtime closure. `-O0` is forced
for this phase because final-module optimizers such as Binaryen do not preserve
relocatable linking metadata.

The Go linker consumes the one aggregate object and owns final data/table
layout, Go dead-code reachability, continuation PCs, Go/native adapters, and
module emission. The observed aggregate currently uses these relocation
families:

- function, global, type, table-index, and table-number LEB relocations;
- absolute, signed, and relative memory-address relocations;
- fixed-width data/table/global relocations; and
- function-offset and section-offset relocations (primarily metadata/debug).

Native imports embedded in the object retain their exact module, field, and
wasm function type. They are emitted directly; there is no extra syscall
wrapper. Linker-synthesized `env` symbols remain unresolved for Go or the final
linker model to provide.

## Current checkpoint and remaining work

The linker relocation/TLS/constructor path, musl startup handoff,
native-signature calls, callback/crosscall path, libc-backed heap regions, and
mixed-pointer-width generator are implemented. The integration test covers
scalars, direct and allocated pointers, strings/slices, pointer-bearing
aggregate values, aggregate results, returned and Go-selected C function
pointers, errno, stack growth, scheduled callbacks, destructor cleanup, and
concurrent pthread cases. It passes feature-enabled validation and 1-, 2-, and
4-CPU distro guests.

The distro's Unix-oriented pure-Go matrix and a patched `mattn/go-sqlite3`
build provide the first ecosystem layer. SQLite needs only platform feature
flags for mmap/WAL and an explicit compact `uintptr_t` result-array transport
at the mixed-layout boundary described above. Its ordinary SQL, backup,
transaction, concurrency, and callback tests exercise the real static-cgo
path.

The remaining work is further validation breadth rather than a known kernel
contract change: classify more packages that assume fork/dlopen/PIE and turn
each newly observed relocation or ABI shape into a minimized regression test.
C++ exceptions, `setjmp`/
`longjmp` across Go frames, pointer-bearing unions, and packed/bitfield
aggregates remain explicit audit cases; they must either be translated safely
or rejected clearly rather than silently mislaid out.

No kernel change is currently required by this design. The native threaded
probe validates the existing clone/Worker mapping, shared memory, atomics,
kernel imports, musl startup, TLS initialization, and libc allocation path.
