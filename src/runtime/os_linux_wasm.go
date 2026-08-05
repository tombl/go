// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
	"internal/runtime/atomic"
	linuxsys "internal/runtime/syscall/linux"
	"unsafe"
)

const (
	_EINTR  = 4
	_EAGAIN = 11
	_ENOMEM = 12
	_EINVAL = 22
	_ENOSYS = 38

	_O_CREAT    = 0x40
	_O_WRONLY   = 0x1
	_O_TRUNC    = 0x200
	_O_NONBLOCK = 0x800
	_O_CLOEXEC  = 0x80000

	_AT_FDCWD = -100

	_SYS_WRITE                  = 64
	_SYS_PIPE2                  = 59
	_SYS_EXIT                   = 93
	_SYS_EXIT_GROUP             = 94
	_SYS_FUTEX                  = 98
	_SYS_CLOCK_NANOSLEEP_TIME64 = 407
	_SYS_CLOCK_GETTIME64        = 403
	_SYS_SCHED_YIELD            = 124
	_SYS_SCHED_GETAFFINITY      = 123
	_SYS_RT_SIGACTION           = 134
	_SYS_RT_SIGPROCMASK         = 135
	_SYS_CLONE                  = 220
	_SYS_GETRANDOM              = 278

	_FUTEX_WAIT_PRIVATE = 128
	_FUTEX_WAKE_PRIVATE = 129

	_CLOCK_REALTIME  = 0
	_CLOCK_MONOTONIC = 1

	_SIGPIPE = 13
	_SIGSEGV = 11

	_SIG_BLOCK   = 0
	_SIG_UNBLOCK = 1
	_SIG_SETMASK = 2

	_SA_SIGINFO  = 0x00000004
	_SA_RESTORER = 0x04000000
	_SA_ONSTACK  = 0x08000000
	_SA_RESTART  = 0x10000000

	_CLONE_VM      = 0x00000100
	_CLONE_FS      = 0x00000200
	_CLONE_FILES   = 0x00000400
	_CLONE_SIGHAND = 0x00000800
	_CLONE_THREAD  = 0x00010000
	_CLONE_SYSVSEM = 0x00040000
)

type timespec64 struct {
	sec  int64
	nsec int64
}

const wasmArgsBufferSize = 256 << 10

var (
	wasmArgsBlob [wasmArgsBufferSize]byte
	wasmArgv     [wasmArgsBufferSize / 8]uintptr
	secureMode   bool
)

//go:wasmimport linux copy_siginfo
//go:nosplit
//go:noescape
func wasmCopySiginfo(buf unsafe.Pointer) int32

func osinit() {
	physPageSize = 64 * 1024
	initBloc()
	blocMax = uintptr(currentMemory()) * physPageSize
	numCPUStartup = getCPUCount()
	getg().m.procid = 2
}

func getCPUCount() int32 {
	var mask [128]byte
	n, _, errno := linuxsys.Syscall6(_SYS_SCHED_GETAFFINITY, 0, uintptr(len(mask)), uintptr(unsafe.Pointer(&mask[0])), 0, 0, 0)
	if errno != 0 || n == 0 || n > uintptr(len(mask)) {
		return 1
	}
	count := int32(0)
	for _, b := range mask[:n] {
		for b != 0 {
			count += int32(b & 1)
			b >>= 1
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

func resetMemoryDataView() {}

func sigpanic() {
	gp := getg()
	if !canpanic() {
		throw("unexpected signal during runtime execution")
	}
	gp.sig = _SIGSEGV
	panicmem()
}

//go:nosplit
func exitThread(wait *atomic.Uint32) {
	wait.Store(0)
	linuxsys.Syscall6(_SYS_EXIT, 0, 0, 0, 0, 0, 0)
	for {
	}
}

type mOS struct {
	waitsema uint32
}

//go:nosplit
func osyield() {
	linuxsys.Syscall6(_SYS_SCHED_YIELD, 0, 0, 0, 0, 0, 0)
}

//go:nosplit
func osyield_no_g() {
	linuxsys.Syscall6(_SYS_SCHED_YIELD, 0, 0, 0, 0, 0, 0)
}

type sigset [2]uint32

var sigset_all = sigset{^uint32(0), ^uint32(0)}

type sigactiont struct {
	sa_handler  uint32
	sa_flags    uint32
	sa_restorer uint32
	sa_mask     sigset
}

var (
	fwdSig      [_NSIG]uintptr
	handlingSig [_NSIG]uint32
)

func mpreinit(mp *m) {
	mp.gsignal = malg(32 * 1024)
	mp.gsignal.m = mp
}

//go:nosplit
func usleep_no_g(usec uint32) { usleep(usec) }

//go:nosplit
func sigsave(mask *sigset) {
	rtsigprocmask(_SIG_SETMASK, nil, mask)
}

//go:nosplit
func msigrestore(mask sigset) {
	rtsigprocmask(_SIG_SETMASK, &mask, nil)
}

//go:nosplit
//go:nowritebarrierrec
func clearSignalHandlers() {
	for sig := uint32(0); sig < _NSIG; sig++ {
		if atomic.Load(&handlingSig[sig]) != 0 {
			setsig(sig, 0)
		}
	}
}

//go:nosplit
func sigblock(exiting bool) {
	rtsigprocmask(_SIG_SETMASK, &sigset_all, nil)
}

func minit()                {}
func unminit()              {}
func mdestroy(mp *m)        {}
func initsig(bool)          {}
func signame(uint32) string { return "" }

const _NSIG = 65

func crash() { abort() }

func wasmMstart()
func wasmSigtramp()

//go:nosplit
func wasmSignalHandler(sig uint32) {
	var info [128]byte
	if wasmCopySiginfo(unsafe.Pointer(&info[0])) != 0 {
		return
	}
	sigsend(sig)
}

//go:nowritebarrierrec
func newosproc(mp *m) {
	const flags = _CLONE_VM | _CLONE_FS | _CLONE_FILES | _CLONE_SIGHAND | _CLONE_THREAD | _CLONE_SYSVSEM
	fn := uintptr(abi.FuncPCABI0(wasmMstart)) >> 16
	for tries := 0; ; tries++ {
		_, _, errno := linuxsys.Syscall6(_SYS_CLONE, fn, uintptr(unsafe.Pointer(mp)), flags, 0, 0, 0)
		if errno == 0 {
			return
		}
		if errno != _EAGAIN || tries == 20 {
			print("runtime: failed to create new OS thread (have ", mcount(), " already; errno=", errno, ")\n")
			throw("newosproc")
		}
		usleep_no_g(1000)
	}
}

//go:nosplit
//go:nowritebarrierrec
func libpreinit() {
	initsig(true)
}

//go:nosplit
//go:nowritebarrierrec
func newosproc0(stacksize uintptr, fn unsafe.Pointer) {
	throw("bad newosproc0")
}

//go:linkname os_sigpipe os.sigpipe
func os_sigpipe() {
	if !signal_ignored(_SIGPIPE) && !sigsend(_SIGPIPE) {
		exit(2)
	}
}

//go:linkname syscall_now syscall.now
func syscall_now() (sec int64, nsec int32) {
	return walltime()
}

//go:nosplit
func cputicks() int64 { return nanotime() }

type gsignalStack struct{}

const preemptMSupported = false

func preemptM(*m) {}

//go:nosplit
func getfp() uintptr { return 0 }

func setProcessCPUProfiler(int32) {}
func setThreadCPUProfiler(int32)  {}
func sigdisable(sig uint32) {
	if sig >= _NSIG {
		return
	}
	atomic.Store(&handlingSig[sig], 0)
	setsig(sig, atomic.Loaduintptr(&fwdSig[sig]))
}
func sigenable(sig uint32) {
	if sig >= _NSIG {
		return
	}
	if atomic.Cas(&handlingSig[sig], 0, 1) {
		atomic.Storeuintptr(&fwdSig[sig], getsig(sig))
		setsig(sig, abi.FuncPCABI0(wasmSigtramp))
	}
}
func sigignore(sig uint32) {
	if sig >= _NSIG {
		return
	}
	atomic.Store(&handlingSig[sig], 0)
	setsig(sig, 1)
}

//go:nosplit
func getsig(sig uint32) uintptr {
	var sa sigactiont
	if errno := rtSigaction(sig, nil, &sa); errno != 0 {
		throw("sigaction read failed")
	}
	if sa.sa_handler <= 1 {
		return uintptr(sa.sa_handler)
	}
	return uintptr(sa.sa_handler) << 16
}

//go:nosplit
func setsig(sig uint32, handler uintptr) {
	var sa sigactiont
	if handler <= 1 {
		sa.sa_handler = uint32(handler)
	} else {
		fn := uint32(handler >> 16)
		sa.sa_handler = fn
		sa.sa_restorer = fn
		sa.sa_flags = _SA_SIGINFO | _SA_RESTORER | _SA_ONSTACK | _SA_RESTART
		sa.sa_mask = sigset_all
	}
	if errno := rtSigaction(sig, &sa, nil); errno != 0 {
		throw("sigaction failed")
	}
}

//go:nosplit
func rtsigprocmask(how int32, new, old *sigset) {
	_, _, errno := linuxsys.Syscall6(_SYS_RT_SIGPROCMASK, uintptr(how), uintptr(unsafe.Pointer(new)), uintptr(unsafe.Pointer(old)), unsafe.Sizeof(sigset{}), 0, 0)
	if errno != 0 {
		throw("sigprocmask failed")
	}
}

//go:nosplit
func rtSigaction(sig uint32, new, old *sigactiont) uintptr {
	_, _, errno := linuxsys.Syscall6(_SYS_RT_SIGACTION, uintptr(sig), uintptr(unsafe.Pointer(new)), uintptr(unsafe.Pointer(old)), unsafe.Sizeof(sigset{}), 0, 0)
	return errno
}

//go:nosplit
func rawResult(r1, errno uintptr) int32 {
	if errno != 0 {
		return -int32(errno)
	}
	return int32(r1)
}

//go:nosplit
func open(name *byte, mode, perm int32) int32 {
	r1, _, errno := linuxsys.Syscall6(linuxsys.SYS_OPENAT, uintptr(^uint32(99)), uintptr(unsafe.Pointer(name)), uintptr(mode), uintptr(perm), 0, 0)
	return rawResult(r1, errno)
}

//go:nosplit
func closefd(fd int32) int32 {
	r1, _, errno := linuxsys.Syscall6(linuxsys.SYS_CLOSE, uintptr(fd), 0, 0, 0, 0, 0)
	return rawResult(r1, errno)
}

//go:nosplit
func read(fd int32, p unsafe.Pointer, n int32) int32 {
	r1, _, errno := linuxsys.Syscall6(linuxsys.SYS_READ, uintptr(fd), uintptr(p), uintptr(n), 0, 0, 0)
	return rawResult(r1, errno)
}

//go:nosplit
func write1(fd uintptr, p unsafe.Pointer, n int32) int32 {
	r1, _, errno := linuxsys.Syscall6(_SYS_WRITE, fd, uintptr(p), uintptr(n), 0, 0, 0)
	return rawResult(r1, errno)
}

//go:nosplit
func fcntl(fd, cmd, arg int32) (int32, int32) {
	r1, _, errno := linuxsys.Syscall6(linuxsys.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg), 0, 0, 0)
	return int32(r1), int32(errno)
}

//go:nosplit
func pipe2(flags int32) (r, w int32, errno int32) {
	var p [2]int32
	_, _, e := linuxsys.Syscall6(_SYS_PIPE2, uintptr(unsafe.Pointer(&p[0])), uintptr(flags), 0, 0, 0, 0)
	return p[0], p[1], int32(e)
}

//go:nosplit
func madvise(unsafe.Pointer, uintptr, int32) int32 { return -_ENOSYS }

//go:nosplit
func exit(code int32) {
	linuxsys.Syscall6(_SYS_EXIT_GROUP, uintptr(code), 0, 0, 0, 0, 0)
	for {
	}
}

//go:nosplit
func usleep(usec uint32) {
	ts := timespec64{sec: int64(usec / 1e6), nsec: int64(usec%1e6) * 1e3}
	linuxsys.Syscall6(_SYS_CLOCK_NANOSLEEP_TIME64, _CLOCK_MONOTONIC, 0, uintptr(unsafe.Pointer(&ts)), 0, 0, 0)
}

//go:nosplit
func futexsleep(addr *uint32, val uint32, ns int64) {
	var tsp unsafe.Pointer
	var ts timespec64
	if ns >= 0 {
		ts.sec = ns / 1e9
		ts.nsec = ns % 1e9
		tsp = unsafe.Pointer(&ts)
	}
	linuxsys.Syscall6(_SYS_FUTEX, uintptr(unsafe.Pointer(addr)), _FUTEX_WAIT_PRIVATE, uintptr(val), uintptr(tsp), 0, 0)
}

//go:nosplit
func futexwakeup(addr *uint32, cnt uint32) {
	linuxsys.Syscall6(_SYS_FUTEX, uintptr(unsafe.Pointer(addr)), _FUTEX_WAKE_PRIVATE, uintptr(cnt), 0, 0, 0)
}

//go:nosplit
func clocktime(clock uintptr) (sec int64, nsec int32) {
	var ts timespec64
	_, _, errno := linuxsys.Syscall6(_SYS_CLOCK_GETTIME64, clock, uintptr(unsafe.Pointer(&ts)), 0, 0, 0, 0)
	if errno != 0 {
		return 0, 0
	}
	return ts.sec, int32(ts.nsec)
}

func walltime() (sec int64, nsec int32) {
	return clocktime(_CLOCK_REALTIME)
}

func nanotime1() int64 {
	sec, nsec := clocktime(_CLOCK_MONOTONIC)
	return sec*1e9 + int64(nsec)
}

func readRandom(r []byte) int {
	if len(r) == 0 {
		return 0
	}
	r1, _, errno := linuxsys.Syscall6(_SYS_GETRANDOM, uintptr(unsafe.Pointer(&r[0])), uintptr(len(r)), 0, 0, 0, 0)
	if errno != 0 {
		return 0
	}
	return int(r1)
}

func goenvs() {
	goenvs_unix()
}

//go:nosplit
func wasmArgsWord(base unsafe.Pointer, off uintptr) uint32 {
	return *(*uint32)(add(base, off))
}

// sysargs converts the kernel's wasm32 process argument blob into the
// 64-bit pointer vector used by GOARCH=wasm.
//
//go:nosplit
func sysargs(_ int32, _ **byte) {
	base := unsafe.Pointer(&wasmArgsBlob[0])
	_, _, errno := linuxsys.Syscall6(linuxsys.SYS_WASM_GET_ARGS, uintptr(base), uintptr(len(wasmArgsBlob)), 0, 0, 0, 0)
	if errno != 0 {
		throw("wasm_get_args failed")
	}
	n := uint64(wasmArgsWord(base, 0)) + 20
	if n > uint64(len(wasmArgsBlob)) {
		throw("invalid process argument blob")
	}

	envc := wasmArgsWord(base, 4)
	argc32 := wasmArgsWord(base, 8)
	argv32 := wasmArgsWord(base, 12)
	envp32 := wasmArgsWord(base, 16)
	base32 := uint32(uintptr(base))
	if argv32 < base32 || envp32 < base32 {
		throw("invalid process argument pointers")
	}

	needed := uint64(argc32) + 1 + uint64(envc) + 1
	if needed > uint64(len(wasmArgv)) {
		throw("too many process arguments")
	}
	out := wasmArgv[:]
	for i := uint32(0); i < argc32; i++ {
		out[i] = uintptr(wasmArgsWord(base, uintptr(argv32-base32)+uintptr(i)*4))
	}
	pos := argc32 + 1
	for i := uint32(0); i < envc; i++ {
		out[pos+i] = uintptr(wasmArgsWord(base, uintptr(envp32-base32)+uintptr(i)*4))
	}
	pos += envc + 1

	auxoff := uintptr(envp32-base32) + uintptr(envc+1)*4
	auxstart := pos
	for {
		if pos+2 > uint32(len(out)) || uint64(auxoff)+8 > n {
			throw("invalid process auxiliary vector")
		}
		tag := wasmArgsWord(base, auxoff)
		val := wasmArgsWord(base, auxoff+4)
		out[pos] = uintptr(tag)
		out[pos+1] = uintptr(val)
		pos += 2
		auxoff += 8
		if tag == 0 {
			break
		}
	}

	argc = int32(argc32)
	argv = (**byte)(unsafe.Pointer(&out[0]))
	for i := auxstart; out[i] != 0; i += 2 {
		tag, val := out[i], out[i+1]
		switch tag {
		case 6: // AT_PAGESZ
			physPageSize = val
		case 23: // AT_SECURE
			secureMode = val == 1
		case 25: // AT_RANDOM
			startupRand = (*[16]byte)(unsafe.Pointer(val))[:]
		}
		archauxv(tag, val)
		vdsoauxv(tag, val)
	}
	auxv = out[auxstart : pos-2 : pos-2]
}
