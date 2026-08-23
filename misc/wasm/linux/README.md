# Go on wasm-linux

This port targets `GOOS=linux GOARCH=wasm`. It combines the upstream Go wasm
compiler and continuation-based stack-switching machinery with the Linux Go
runtime, scheduler, and syscall surface. The target is a normal, statically
linked Linux process running in shared WebAssembly linear memory; it is not the
`js/wasm` or `wasip1/wasm` host ABI.

## Threading model

The port keeps Go's ordinary G-M-P scheduler:

```
goroutines (G) -> logical processors (P) -> OS threads (M)
               -> Linux clone tasks -> Web Workers/wasm instances
```

An M is a Linux task backed by one Web Worker and one instance of the userspace
wasm module. Goroutines are multiplexed over a small set of Ms; they are never
mapped one-to-one to Workers. The heap, scheduler state, G stacks, and M
structures live in shared linear memory. Instance globals such as SP, g, CTXT,
and PAUSE are thread-local because each Worker has its own module instance.

`newosproc` uses the platform's callback form of `clone`. The child callback
initializes the new instance's SP and g from the M's g0 and enters the stock Go
`mstart` path. Runtime synchronization uses wasm atomics and Linux futexes.
`GOMAXPROCS` therefore controls real parallel execution, and the integration
test requires two kernel CPUs and verifies progress on a second P while another
P is occupied by a call-free loop.

### Stack switching

WebAssembly cannot replace the native call stack directly. The port retains
the upstream wasm continuation transform: compiled functions take a logical
block ID and return an unwind flag, while packed function/block resume PCs are
stored on the Go linear-memory stack. `gogo` changes g and SP, unwinds the wasm
call chain, and the permanent `wasm_pc_f_loop` reconstructs the logical call
chain with indirect calls and branch tables.

Stack checks use both `g.stackguard0` and the authoritative `g.stack.lo`
boundary. The latter is important after a goroutine migrates between Ms: it
prevents a stale guard observation from allowing a large frame to enter an
exhausted stack. `TestConcurrentStackGrowth` exercises concurrent deep growth
and migration.

### Preemption

Asynchronous Go preemption is not supported. Function prologues and normal Go
safe points still provide cooperative scheduling and stack growth, but the
compiler does not insert general safe points into call-free loops. Such a loop
can pin one P indefinitely. With more than one P, other goroutines can continue
on other Ms; with one P, they cannot. This is a platform limitation and must
remain a documented behavioral caveat.

## Memory and Linux ABI

Runtime heap reservation and growth use WebAssembly `memory.grow`, through the
runtime's sbrk-like allocator. The runtime does not use `mmap`, `munmap`, or
`madvise` to manage the Go heap. Public mmap-family calls are unsupported and
must fail rather than pretending to create independently mapped regions.

Go pointers and `uintptr` are 64 bits, while the kernel and musl userspace ABI
are ILP32. Syscall arguments that contain pointers therefore use explicit wire
structures at the syscall boundary. This is required for structures such as
`iovec`, `msghdr`, signal actions, process argument vectors, and callback clone
arguments. Time-related calls must use the kernel's time64 syscall variants.

The supported runtime contract includes ordinary file and path operations,
pipes, sockets and Go netpoll through epoll, clocks and timers, vectored I/O,
futexes, process spawn/exec/wait, and asynchronous signal delivery. The port
does not require `fork`: `os/exec` uses callback `clone` with `CLONE_VM` and
`CLONE_VFORK`, followed by `execve` in the child callback.

Synchronous fault-to-signal translation is not available. The platform can
deliver asynchronous signals at its supported boundaries, but invalid memory,
divide, or other wasm traps cannot currently be converted into Go's usual
SIGSEGV/SIGFPE panic path.

## Current unsupported features

- dynamic cgo and non-static C linkage
- the race detector
- asynchronous Go preemption
- synchronous fault-to-signal translation
- `fork`, `vfork`, and mmap-family memory mappings
- PIE/PIC, shared libraries, plugins, and non-static build modes
- Linux `SysProcAttr` features beyond the explicitly implemented spawn subset

These are feature exclusions, not reasons to weaken supported syscalls such as
epoll, time, timerfd, readv, or writev.

## Static cgo

Statically linked cgo uses wasm-ld for archive selection and one relocatable
aggregate, then the Go linker owns final module emission. musl crt1 owns
startup, libc owns `memory.grow`, and generated cgo wrappers translate between
the 64-bit Go pointer ABI and wasm32 C pointers. Calls switch to g0; callbacks
from both Go-created and C-created pthreads attach to the scheduler and may
grow/copy Go stacks.

The focused integration program covers pointer-bearing structs and arrays,
aggregate results, function pointers, errno, libc allocation, callbacks,
pthread teardown, GC, and SMP execution on 1-, 2-, and 4-CPU guests. Remaining
work is ecosystem and unusual-ABI validation (notably packed/bitfield
aggregates, pointer-bearing unions, and C++ unwinding), before static cgo can be
called broadly production-ready. Dynamic loading remains a separate and
unsupported platform feature.

The current implementation checkpoint and validated VM probe are documented in
`cgo-prototype/DESIGN.md`.

## Validation

From a built Go tree, host-side compiler/linker tests and target standard
library compilation can be run normally. Guest behavior is exercised by:

```
WASM_LINUX_DISTRO=/path/to/distro ./misc/wasm/linux/run.bash
```

The integration suite covers startup, arguments and environment, files,
networking/netpoll, timers, signals, process spawn, native threads, atomics,
memory growth, cooperative-preemption behavior, and concurrent stack growth.
The distro additionally runs selected upstream `x/sys`, `x/net`, `x/crypto`,
and application tests in a two-core installed VM.

An unmodified `go test std` is not yet a valid target suite: several upstream
Linux runtime test support files assume mmap helpers and synchronous fault
handling that do not exist on `linux/wasm`. Those tests need wasm-specific
build constraints or replacements, followed by broader guest execution, before
the port can claim complete upstream test-suite coverage.
