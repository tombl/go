# Static cgo prototype

This directory is the first `linux/wasm` static-cgo probe. It deliberately uses
only fixed-width scalar arguments so the object and linker path can be tested
independently of pointer marshalling.

The prototype establishes that:

- the distro's `wasm32-unknown-linux-musl-clang` compiles the cgo probes and C
  sources;
- cmd/cgo can read DWARF from relocatable wasm custom sections and generate the
  Go and C wrappers for `prototype_add32`;
- Go values exposed to C need a 64-bit representation even though C pointers
  are 32-bit; and
- the remaining build boundary is relocatable wasm ingestion or emission.

With the prototype changes, an automatic link reaches the wasm linker's
unimplemented external-relocation path. Forcing `-linkmode=internal` instead
reports the Clang objects as an unrecognized format. A final Go executable also
cannot be passed to `wasm-ld -r`: it has no standard `linking` or `reloc.*`
sections and wasm-ld reports that it is not a relocatable wasm file.

The next prototype step is to extend `internal/wasmobj` with the standard wasm
linking section, symbol table, data-segment metadata, and relocation records,
then load one partially linked C object into cmd/link while retaining the Go
linker as the final module writer.
