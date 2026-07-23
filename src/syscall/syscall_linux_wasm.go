// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

import (
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
