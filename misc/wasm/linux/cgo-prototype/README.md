# Static cgo prototype

This directory is the first `linux/wasm` static-cgo probe. It deliberately uses
only fixed-width scalar arguments so the object and linker path can be tested
independently of pointer marshalling.

The prototype now establishes that:

- the distro's Clang, targeted at `wasm32-unknown-linux-musl` with its sysroot,
  compiles the cgo probes and C sources;
- cmd/cgo can read DWARF from relocatable wasm custom sections and generate the
  Go and C wrappers for `prototype_add32`;
- Go values exposed to C need a 64-bit representation even though C pointers
  are 32-bit; and
- cmd/link can ingest generated relocatable wasm objects, retain their native
  wasm signatures, and invoke the target C driver once at whole-program link
  time with `-r`;
- wasm-ld successfully selects the required crt1, static musl, pthread, and
  compiler-runtime archive members while leaving Go bridge symbols unresolved;
- the Go linker preserves the native `linux.syscall`, `linux.get_thread_area`,
  and `linux.copy_siginfo` imports from the resulting object; and
- C's `__tls_base` and Go's `g` register can be independent globals in each
  WebAssembly instance.

The build is deliberately kept on Go's internal-link path. linux/wasm is
static-only, so cmd/go skips its dynamic-import probe and cmd/link treats wasm
host objects from every cgo package as internal objects. New serialized Go
relocation kinds are appended to the existing enumeration so old Go object
files retain their numeric ABI.

The current boundary is now explicit rather than archive-resolution failure:

- wasm's existing `runtime.asmcgocall`, `crosscall2`, and C-to-Go entry stubs
  are `UNDEF`, and a C call cannot target Go's `(i32) -> i32` continuation ABI
  directly; it needs a native wasm-signature adapter;
- the Go linker must synthesize `__wasm_init_tls`, init/fini array boundaries,
  constructor dispatch, and the C-owned startup handoff;
- relocations inside C data segments and the remaining archive relocation
  families must be applied for initialized function pointers and globals;
- non-empty C TLS segments still need final layout and TLS-relative
  relocations (the per-instance base global is already modeled); and
- the cgo runtime allocator must request aligned regions from libc instead of
  directly executing `memory.grow`;
- the emitted module must pass feature-enabled wasm validation before it is
  run in the distro VM.

`linkprobe/native_stubs.c` is deliberately not part of the Go package. It
supplies temporary bridge symbols so the native half can be finalized and run
independently. The probe has passed the distro's real two-CPU VM while creating
four pthreads, checking zero-initialized per-instance C TLS, concurrently
allocating and freeing memory through musl, and updating a shared wasm atomic.

See [DESIGN.md](DESIGN.md) for the decisions and remaining implementation
sequence.
