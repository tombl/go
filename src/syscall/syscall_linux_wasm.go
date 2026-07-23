// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

import (
	"internal/abi"
	"runtime"
	"unsafe"
)

//go:nosplit
func rawSyscallNoError(trap, a1, a2, a3 uintptr) (r1, r2 uintptr) {
	r1, r2, _ = RawSyscall(trap, a1, a2, a3)
	return
}

// The generic Linux fork/exec path relies on a syscall returning twice.
// Linux/wasm exposes callback-based clone instead, so fail cleanly until the
// exec path supplies an explicit child entrypoint.
//
//go:nosplit
func rawVforkSyscall(trap, a1, a2, a3 uintptr) (r1 uintptr, err Errno) {
	return 0, ENOSYS
}

const wasmExecStackSize = 64 << 10

var wasmExecStack [wasmExecStackSize]byte

type wasmExecArgs struct {
	stack     uintptr
	path      *byte
	argv      *uint32
	envv      *uint32
	dir       *byte
	chroot    *byte
	files     *int32
	fileCount uint32
	errorFD   int32
	sigmask   [2]uint32
}

type wasmSigaction struct {
	handler  uint32
	flags    uint32
	restorer uint32
	mask     [2]uint32
}

func wasmExecChild()

func wasmPointerVector(pointers []*byte) []uint32 {
	wire := make([]uint32, len(pointers))
	for i, p := range pointers {
		wire[i] = wasmPointer(unsafe.Pointer(p))
	}
	return wire
}

func wasmExecve(path *byte, argv, envv []*byte) error {
	wargv := wasmPointerVector(argv)
	wenvv := wasmPointerVector(envv)
	_, _, errno := RawSyscall(SYS_EXECVE,
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&wargv[0])),
		uintptr(unsafe.Pointer(&wenvv[0])))
	runtime.KeepAlive(argv)
	runtime.KeepAlive(envv)
	if errno != 0 {
		return errno
	}
	return nil
}

func wasmForkAndExec(argv0 *byte, argv, envv []*byte, chroot, dir *byte, attr *ProcAttr, sys *SysProcAttr, pipe int) (pid int, err Errno) {
	if sys.Credential != nil || sys.Ptrace || sys.Setsid || sys.Setpgid || sys.Setctty || sys.Noctty || sys.Foreground ||
		sys.Pdeathsig != 0 || sys.Cloneflags != 0 || sys.Unshareflags != 0 || len(sys.UidMappings) != 0 ||
		len(sys.GidMappings) != 0 || len(sys.AmbientCaps) != 0 || sys.UseCgroupFD || sys.PidFD != nil {
		return 0, EOPNOTSUPP
	}

	wargv := wasmPointerVector(argv)
	wenvv := wasmPointerVector(envv)
	childFiles := make([]int32, len(attr.Files))
	minFD := len(attr.Files) + 1
	if minFD <= pipe {
		minFD = pipe + 1
	}
	for i, fd := range attr.Files {
		if fd == ^uintptr(0) {
			childFiles[i] = -1
			continue
		}
		r, _, errno := Syscall(SYS_FCNTL, fd, F_DUPFD_CLOEXEC, uintptr(minFD))
		if errno != 0 {
			for _, duplicated := range childFiles[:i] {
				if duplicated >= 0 {
					Close(int(duplicated))
				}
			}
			return 0, errno
		}
		childFiles[i] = int32(r)
		if int(r) >= minFD {
			minFD = int(r) + 1
		}
	}
	childErrorFD, _, errno := Syscall(SYS_FCNTL, uintptr(pipe), F_DUPFD_CLOEXEC, uintptr(minFD))
	if errno != 0 {
		for _, duplicated := range childFiles {
			if duplicated >= 0 {
				Close(int(duplicated))
			}
		}
		return 0, errno
	}

	args := wasmExecArgs{
		stack:     uintptr(unsafe.Pointer(&wasmExecStack[len(wasmExecStack)-16])),
		path:      argv0,
		argv:      &wargv[0],
		envv:      &wenvv[0],
		dir:       dir,
		chroot:    chroot,
		fileCount: uint32(len(childFiles)),
		errorFD:   int32(childErrorFD),
	}
	if len(childFiles) != 0 {
		args.files = &childFiles[0]
	}

	// Keep the child from running a Go signal handler before exec. Pinning
	// this goroutine makes the saved and restored masks belong to the same
	// kernel thread, while CLONE_VFORK keeps the parent stopped until the
	// child has either execed or exited.
	runtime.LockOSThread()
	var oldMask [2]uint32
	_, _, errno = RawSyscall6(SYS_RT_SIGPROCMASK, 2, 0, uintptr(unsafe.Pointer(&oldMask[0])), unsafe.Sizeof(oldMask), 0, 0)
	if errno == 0 {
		allSignals := [2]uint32{^uint32(0), ^uint32(0)}
		_, _, errno = RawSyscall6(SYS_RT_SIGPROCMASK, 2, uintptr(unsafe.Pointer(&allSignals[0])), 0, unsafe.Sizeof(allSignals), 0, 0)
	}
	if errno != 0 {
		runtime.UnlockOSThread()
		for _, duplicated := range childFiles {
			if duplicated >= 0 {
				Close(int(duplicated))
			}
		}
		Close(int(childErrorFD))
		return 0, errno
	}
	args.sigmask = oldMask

	fn := uintptr(abi.FuncPCABI0(wasmExecChild)) >> 16
	const flags = CLONE_VM | CLONE_VFORK | uintptr(SIGCHLD)
	r1, _, errno := Syscall6(SYS_CLONE, fn, uintptr(unsafe.Pointer(&args)), flags, 0, 0, 0)
	RawSyscall6(SYS_RT_SIGPROCMASK, 2, uintptr(unsafe.Pointer(&oldMask[0])), 0, unsafe.Sizeof(oldMask), 0, 0)
	runtime.UnlockOSThread()
	for _, duplicated := range childFiles {
		if duplicated >= 0 {
			Close(int(duplicated))
		}
	}
	Close(int(childErrorFD))
	runtime.KeepAlive(argv)
	runtime.KeepAlive(envv)
	runtime.KeepAlive(wargv)
	runtime.KeepAlive(wenvv)
	runtime.KeepAlive(childFiles)
	runtime.KeepAlive(args)
	if errno != 0 {
		return 0, errno
	}
	return int(r1), 0
}

//go:nosplit
//go:norace
func wasmExecChildGo(args *wasmExecArgs) {
	files := unsafe.Slice(args.files, args.fileCount)
	for target, source := range files {
		if source < 0 {
			RawSyscall(SYS_CLOSE, uintptr(target), 0, 0)
			continue
		}
		if _, _, errno := RawSyscall(SYS_DUP3, uintptr(source), uintptr(target), 0); errno != 0 {
			wasmExecChildError(args, errno)
		}
		RawSyscall(SYS_CLOSE, uintptr(source), 0, 0)
	}
	if args.chroot != nil {
		if _, _, errno := RawSyscall(SYS_CHROOT, uintptr(unsafe.Pointer(args.chroot)), 0, 0); errno != 0 {
			wasmExecChildError(args, errno)
		}
	}
	if args.dir != nil {
		if _, _, errno := RawSyscall(SYS_CHDIR, uintptr(unsafe.Pointer(args.dir)), 0, 0); errno != 0 {
			wasmExecChildError(args, errno)
		}
	}
	// Caught signal handlers point into the parent's Go runtime and cannot
	// safely run in this pre-exec child. Preserve SIG_IGN, as exec does, but
	// reset every caught disposition before unblocking signals.
	var oldAction wasmSigaction
	var defaultAction wasmSigaction
	for sig := uintptr(1); sig <= 64; sig++ {
		if sig == uintptr(SIGKILL) || sig == uintptr(SIGSTOP) {
			continue
		}
		_, _, errno := RawSyscall6(SYS_RT_SIGACTION, sig, 0, uintptr(unsafe.Pointer(&oldAction)), unsafe.Sizeof(oldAction.mask), 0, 0)
		if errno == EINVAL {
			continue
		}
		if errno != 0 {
			wasmExecChildError(args, errno)
		}
		if oldAction.handler > 1 {
			if _, _, errno := RawSyscall6(SYS_RT_SIGACTION, sig, uintptr(unsafe.Pointer(&defaultAction)), 0, unsafe.Sizeof(defaultAction.mask), 0, 0); errno != 0 {
				wasmExecChildError(args, errno)
			}
		}
	}
	if _, _, errno := RawSyscall6(SYS_RT_SIGPROCMASK, 2, uintptr(unsafe.Pointer(&args.sigmask[0])), 0, unsafe.Sizeof(args.sigmask), 0, 0); errno != 0 {
		wasmExecChildError(args, errno)
	}
	_, _, errno := RawSyscall(SYS_EXECVE, uintptr(unsafe.Pointer(args.path)), uintptr(unsafe.Pointer(args.argv)), uintptr(unsafe.Pointer(args.envv)))
	wasmExecChildError(args, errno)
}

//go:nosplit
//go:norace
func wasmExecChildError(args *wasmExecArgs, errno Errno) {
	RawSyscall(SYS_WRITE, uintptr(args.errorFD), uintptr(unsafe.Pointer(&errno)), unsafe.Sizeof(errno))
	RawSyscall(SYS_EXIT, 253, 0, 0)
	for {
	}
}

const (
	_SYS_setgroups  = SYS_SETGROUPS
	_SYS_clone3     = 435
	_SYS_faccessat2 = 439
	_SYS_fchmodat2  = 452
)

//sys	EpollWait(epfd int, events []EpollEvent, msec int) (n int, err error) = SYS_EPOLL_PWAIT
//sys	Fchown(fd int, uid int, gid int) (err error)
//sys	Fstat(fd int, stat *Stat_t) (err error)
//sys	fstatat(dirfd int, path string, stat *Stat_t, flags int) (err error)

func Fstatat(fd int, path string, stat *Stat_t, flags int) error {
	return fstatat(fd, path, stat, flags)
}

//sys	Fstatfs(fd int, buf *Statfs_t) (err error)
//sys	Ftruncate(fd int, length int64) (err error)
//sysnb	Getegid() (egid int)
//sysnb	Geteuid() (euid int)
//sysnb	Getgid() (gid int)
//sysnb	Getuid() (uid int)
//sys	Listen(s int, n int) (err error)
//sys	pread(fd int, p []byte, offset int64) (n int, err error) = SYS_PREAD64
//sys	pwrite(fd int, p []byte, offset int64) (n int, err error) = SYS_PWRITE64
//sys	renameat2(olddirfd int, oldpath string, newdirfd int, newpath string, flags uint) (err error)
//sys	Seek(fd int, offset int64, whence int) (off int64, err error) = SYS_LSEEK
//sys	sendfile(outfd int, infd int, offset *int64, count int) (written int, err error)
//sys	Setfsgid(gid int) (err error)
//sys	Setfsuid(uid int) (err error)
//sys	Shutdown(fd int, how int) (err error)
//sys	Splice(rfd int, roff *int64, wfd int, woff *int64, len int, flags int) (n int64, err error)

func Renameat(olddirfd int, oldpath string, newdirfd int, newpath string) (err error) {
	return renameat2(olddirfd, oldpath, newdirfd, newpath, 0)
}

func Stat(path string, stat *Stat_t) (err error) {
	return fstatat(_AT_FDCWD, path, stat, 0)
}

func Lchown(path string, uid int, gid int) (err error) {
	return Fchownat(_AT_FDCWD, path, uid, gid, _AT_SYMLINK_NOFOLLOW)
}

func Lstat(path string, stat *Stat_t) (err error) {
	return fstatat(_AT_FDCWD, path, stat, _AT_SYMLINK_NOFOLLOW)
}

//sys	Statfs(path string, buf *Statfs_t) (err error)
//sys	SyncFileRange(fd int, off int64, n int64, flags int) (err error)
//sys	Truncate(path string, length int64) (err error)
//sys	accept4(s int, rsa *RawSockaddrAny, addrlen *_Socklen, flags int) (fd int, err error)
//sys	bind(s int, addr unsafe.Pointer, addrlen _Socklen) (err error)
//sys	connect(s int, addr unsafe.Pointer, addrlen _Socklen) (err error)
//sysnb	getgroups(n int, list *_Gid_t) (nn int, err error)
//sys	getsockopt(s int, level int, name int, val unsafe.Pointer, vallen *_Socklen) (err error)
//sys	setsockopt(s int, level int, name int, val unsafe.Pointer, vallen uintptr) (err error)
//sysnb	socket(domain int, typ int, proto int) (fd int, err error)
//sysnb	socketpair(domain int, typ int, proto int, fd *[2]int32) (err error)
//sysnb	getpeername(fd int, rsa *RawSockaddrAny, addrlen *_Socklen) (err error)
//sysnb	getsockname(fd int, rsa *RawSockaddrAny, addrlen *_Socklen) (err error)
//sys	recvfrom(fd int, p []byte, flags int, from *RawSockaddrAny, fromlen *_Socklen) (n int, err error)
//sys	sendto(s int, buf []byte, flags int, to unsafe.Pointer, addrlen _Socklen) (err error)
//sys	mmap(addr uintptr, length uintptr, prot int, flags int, fd int, offset int64) (xaddr uintptr, err error)

type sigset_t struct {
	X__val [32]uint32
}

type wasmIovec struct {
	Base uint32
	Len  uint32
}

type wasmMsghdr struct {
	Name       uint32
	Namelen    uint32
	Iov        uint32
	Iovlen     int32
	Control    uint32
	Controllen uint32
	Flags      int32
}

func wasmPointer(p unsafe.Pointer) uint32 {
	return uint32(uintptr(p))
}

func setsockoptSockFprog(fd int, program *SockFprog) error {
	wprogram := struct {
		Len    uint16
		_pad   uint16
		Filter uint32
	}{
		Len:    program.Len,
		Filter: wasmPointer(unsafe.Pointer(program.Filter)),
	}
	return setsockopt(fd, SOL_SOCKET, SO_ATTACH_FILTER, unsafe.Pointer(&wprogram), unsafe.Sizeof(wprogram))
}

func wasmMsghdrFor(msg *Msghdr) (wasmMsghdr, []wasmIovec, error) {
	if msg.Iovlen < 0 {
		return wasmMsghdr{}, nil, EINVAL
	}
	iovecs := make([]wasmIovec, int(msg.Iovlen))
	if len(iovecs) != 0 {
		if msg.Iov == nil {
			return wasmMsghdr{}, nil, EFAULT
		}
		goIovecs := unsafe.Slice(msg.Iov, len(iovecs))
		for i := range goIovecs {
			iovecs[i] = wasmIovec{
				Base: wasmPointer(unsafe.Pointer(goIovecs[i].Base)),
				Len:  goIovecs[i].Len,
			}
		}
	}
	wmsg := wasmMsghdr{
		Name:       wasmPointer(unsafe.Pointer(msg.Name)),
		Namelen:    msg.Namelen,
		Iovlen:     msg.Iovlen,
		Control:    wasmPointer(unsafe.Pointer(msg.Control)),
		Controllen: msg.Controllen,
		Flags:      msg.Flags,
	}
	if len(iovecs) != 0 {
		wmsg.Iov = wasmPointer(unsafe.Pointer(&iovecs[0]))
	}
	return wmsg, iovecs, nil
}

func recvmsg(s int, msg *Msghdr, flags int) (n int, err error) {
	wmsg, iovecs, err := wasmMsghdrFor(msg)
	if err != nil {
		return 0, err
	}
	r0, _, e1 := Syscall(SYS_RECVMSG, uintptr(s), uintptr(unsafe.Pointer(&wmsg)), uintptr(flags))
	runtime.KeepAlive(iovecs)
	runtime.KeepAlive(msg)
	msg.Namelen = wmsg.Namelen
	msg.Controllen = wmsg.Controllen
	msg.Flags = wmsg.Flags
	if e1 != 0 {
		return int(r0), errnoErr(e1)
	}
	return int(r0), nil
}

func sendmsg(s int, msg *Msghdr, flags int) (n int, err error) {
	wmsg, iovecs, err := wasmMsghdrFor(msg)
	if err != nil {
		return 0, err
	}
	r0, _, e1 := Syscall(SYS_SENDMSG, uintptr(s), uintptr(unsafe.Pointer(&wmsg)), uintptr(flags))
	runtime.KeepAlive(iovecs)
	runtime.KeepAlive(msg)
	if e1 != 0 {
		return int(r0), errnoErr(e1)
	}
	return int(r0), nil
}

//sys	pselect(nfd int, r *FdSet, w *FdSet, e *FdSet, timeout *Timespec, sigmask *sigset_t) (n int, err error) = SYS_PSELECT6

func Select(nfd int, r *FdSet, w *FdSet, e *FdSet, timeout *Timeval) (n int, err error) {
	var ts *Timespec
	if timeout != nil {
		ts = &Timespec{Sec: timeout.Sec, Nsec: int32(timeout.Usec * 1000)}
	}
	return pselect(nfd, r, w, e, ts, nil)
}

//sysnb	Gettimeofday(tv *Timeval) (err error)

func setTimespec(sec, nsec int64) Timespec {
	return Timespec{Sec: sec, Nsec: int32(nsec)}
}

func setTimeval(sec, usec int64) Timeval {
	return Timeval{Sec: sec, Usec: usec}
}

func futimesat(dirfd int, path string, tv *[2]Timeval) (err error) {
	if tv == nil {
		return utimensat(dirfd, path, nil, 0)
	}

	ts := []Timespec{
		NsecToTimespec(TimevalToNsec(tv[0])),
		NsecToTimespec(TimevalToNsec(tv[1])),
	}
	return utimensat(dirfd, path, (*[2]Timespec)(unsafe.Pointer(&ts[0])), 0)
}

func Time(t *Time_t) (Time_t, error) {
	var tv Timeval
	err := Gettimeofday(&tv)
	if err != nil {
		return 0, err
	}
	if t != nil {
		*t = Time_t(tv.Sec)
	}
	return Time_t(tv.Sec), nil
}

func Utime(path string, buf *Utimbuf) error {
	if buf == nil {
		return Utimes(path, nil)
	}
	tv := []Timeval{
		{Sec: buf.Actime},
		{Sec: buf.Modtime},
	}
	return Utimes(path, tv)
}

func utimes(path string, tv *[2]Timeval) (err error) {
	if tv == nil {
		return utimensat(_AT_FDCWD, path, nil, 0)
	}

	ts := []Timespec{
		NsecToTimespec(TimevalToNsec(tv[0])),
		NsecToTimespec(TimevalToNsec(tv[1])),
	}
	return utimensat(_AT_FDCWD, path, (*[2]Timespec)(unsafe.Pointer(&ts[0])), 0)
}

func (iov *Iovec) SetLen(length int) {
	iov.Len = uint32(length)
}

func (msghdr *Msghdr) SetControllen(length int) {
	msghdr.Controllen = uint32(length)
}

func (cmsg *Cmsghdr) SetLen(length int) {
	cmsg.Len = uint32(length)
}

func InotifyInit() (fd int, err error) {
	return InotifyInit1(0)
}

//sys	ppoll(fds *pollFd, nfds int, timeout *Timespec, sigmask *sigset_t) (n int, err error)

func Pause() error {
	_, err := ppoll(nil, 0, nil, nil)
	return err
}
