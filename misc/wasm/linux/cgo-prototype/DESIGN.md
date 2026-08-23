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
The linker still needs to lay out non-empty `.tdata`/`.tbss`, synthesize
`__tls_size` and `__tls_align`, and implement TLS-relative relocations.

## Pointer boundary

The Go ABI remains pointer-width 64 and the C ABI remains wasm32 pointer-width
32. Generated cgo adapters zero-extend C pointers on entry to Go and narrow Go
pointers on entry to C.

All genuine C pointers necessarily fit because they address a memory32 linear
memory capped at 4 GiB. Nevertheless, Go-to-C narrowing must reject nonzero
high bits. `unsafe.Pointer` and integer conversions can manufacture an invalid
64-bit value; silently truncating it would alias an unrelated low address.
The check is an ABI safety invariant, not an allocator fast-path check.

Aggregate field translation stays in generated C/Go wrappers. WebAssembly's
strict function types mean that no generic variadic or untyped trampoline can
substitute for signature-specific adapters.

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

## Implementation order

1. Finish DATA relocations, init/fini boundaries, constructor collection, and
   non-empty LLVM TLS layout.
2. Add native-signature `asmcgocall`, callback/crosscall, and startup adapters;
   prohibit longjmp and exceptions from crossing Go frames.
3. Add the libc-backed cgo heap-region provider and remove direct Go
   `memory.grow` from cgo builds.
4. Validate scalar, pointer, struct, string/slice, callback, errno, destructor,
   and concurrent pthread cases in the distro VM.
5. Run the pure-Go standard-library and ecosystem suites unchanged, then add
   cgo-enabled packages and stress tests on 1-, 2-, and 4-CPU guests.

No kernel change is currently required by this design. The native threaded
probe validates the existing clone/Worker mapping, shared memory, atomics,
kernel imports, musl startup, TLS initialization, and libc allocation path.
