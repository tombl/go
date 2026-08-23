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
- cmd/link can ingest the generated relocatable wasm objects, retain their
  native wasm signatures, lay out C text and data with Go text and data, and
  encode the C relocation families emitted by the current runtime objects.

The build is deliberately kept on Go's internal-link path. linux/wasm is
static-only, so cmd/go skips its dynamic-import probe and cmd/link treats wasm
host objects from every cgo package as internal objects. New serialized Go
relocation kinds are appended to the existing enumeration so old Go object
files retain their numeric ABI.

The current boundary is now explicit rather than an object-format failure:

- the internal linker does not yet search and ingest the sysroot's static musl,
  pthread, and compiler-runtime archives, so runtime/cgo's libc references are
  unresolved;
- wasm's existing `runtime.asmcgocall`, `crosscall2`, and C-to-Go entry stubs
  are `UNDEF`, and a C call cannot target Go's `(i32) -> i32` continuation ABI
  directly; it needs a native wasm-signature adapter;
- relocations inside C data segments (not just function bodies) still need to
  be applied for initialized C function pointers and address-bearing globals;
- C TLS and constructors have not been modeled; and
- the emitted module must pass feature-enabled wasm validation before it is
  run in the distro VM.

The next implementation slice should therefore ingest the exact static archive
closure selected by the C driver, then add `asmcgocall` and C-to-Go adapters.
That keeps libc selection in the distro toolchain while retaining Go's linker
as the owner of the final continuation-PC encoding.
