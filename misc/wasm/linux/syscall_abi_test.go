// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && wasm

package linuxwasmtest

import (
	"io"
	"os"
	"syscall"
	"testing"
	"time"
)

// The wasm32 kernel takes 32-bit argument slots and has the asm-generic
// 32-bit entries, while Go's types on linux/wasm have musl's time64 layout.
// These calls cross that boundary with 64-bit offsets or time structures.

func TestSyscallABIFileOffsets(t *testing.T) {
	f, err := os.Create("/go-linux-wasm-abi")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, err := f.WriteString("hello"); err != nil {
		t.Fatal(err)
	}
	if off, err := f.Seek(0, io.SeekCurrent); err != nil || off != 5 {
		t.Errorf("Seek(0, SeekCurrent) = %d, %v; want 5", off, err)
	}
	if off, err := f.Seek(2, io.SeekStart); err != nil || off != 2 {
		t.Errorf("Seek(2, SeekStart) = %d, %v; want 2", off, err)
	}

	const big = 5 << 30
	if err := f.Truncate(big); err != nil {
		t.Fatal(err)
	}
	if st, err := f.Stat(); err != nil || st.Size() != big {
		t.Errorf("size after Truncate(%d) = %d, %v", int64(big), st.Size(), err)
	}
	if off, err := f.Seek(0, io.SeekEnd); err != nil || off != big {
		t.Errorf("Seek(0, SeekEnd) = %d, %v; want %d", off, err, int64(big))
	}

	const far = 6 << 30
	if n, err := f.WriteAt([]byte("x"), far); err != nil || n != 1 {
		t.Fatalf("WriteAt(%d) = %d, %v", int64(far), n, err)
	}
	if st, err := f.Stat(); err != nil || st.Size() != far+1 {
		t.Errorf("size after WriteAt(%d) = %d, %v", int64(far), st.Size(), err)
	}
	buf := make([]byte, 1)
	if n, err := f.ReadAt(buf, far); err != nil || n != 1 || buf[0] != 'x' {
		t.Errorf("ReadAt(%d) = %d, %q, %v", int64(far), n, buf, err)
	}

	if err := f.Truncate(0); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Fallocate(int(f.Fd()), 0, 0, 4096); err == syscall.EOPNOTSUPP {
		t.Log("Fallocate: not supported by this filesystem")
	} else if err != nil {
		t.Errorf("Fallocate: %v", err)
	} else if st, err := f.Stat(); err != nil || st.Size() != 4096 {
		t.Errorf("size after Fallocate = %d, %v", st.Size(), err)
	}
	if err := syscall.SyncFileRange(int(f.Fd()), 0, 4096, 2); err != nil {
		t.Errorf("SyncFileRange: %v", err)
	}
	// Invalid flags must reach the kernel as flags, not as part of the range.
	if err := syscall.SyncFileRange(int(f.Fd()), 0, 4096, 0x80); err != syscall.EINVAL {
		t.Errorf("SyncFileRange with invalid flags = %v; want EINVAL", err)
	}

	var fs syscall.Statfs_t
	if err := syscall.Fstatfs(int(f.Fd()), &fs); err != nil || fs.Bsize <= 0 {
		t.Errorf("Fstatfs = %+v, %v", fs, err)
	}
	if err := syscall.Statfs("/", &fs); err != nil || fs.Bsize <= 0 {
		t.Errorf("Statfs = %+v, %v", fs, err)
	}
}

func near(t *testing.T, name string, got, want time.Time) {
	t.Helper()
	if d := got.Sub(want); d < -10*time.Second || d > 10*time.Second {
		t.Errorf("%s = %v; want about %v", name, got, want)
	}
}

func TestSyscallABITimes(t *testing.T) {
	now := time.Now()

	var tv syscall.Timeval
	if err := syscall.Gettimeofday(&tv); err != nil {
		t.Fatal(err)
	}
	if tv.Usec < 0 || tv.Usec >= 1e6 {
		t.Errorf("Gettimeofday Usec = %d", tv.Usec)
	}
	near(t, "Gettimeofday", time.Unix(tv.Sec, tv.Usec*1e3), now)

	const path = "/go-linux-wasm-abi-times"
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "Stat ModTime", fi.ModTime(), now)
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		t.Fatal(err)
	}
	near(t, "Lstat Mtim", time.Unix(st.Mtim.Unix()), now)
	if st.Size != int64(len(payload)) || st.Mode&syscall.S_IFMT != syscall.S_IFREG {
		t.Errorf("Lstat = %+v", st)
	}

	want := time.Unix(1000000000, 123456789)
	if err := os.Chtimes(path, want, want); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || !fi.ModTime().Equal(want) {
		t.Errorf("ModTime after Chtimes = %v, %v; want %v", fi.ModTime(), err, want)
	}

	// An invalid timeval must be rejected, not truncated into a valid time.
	if err := syscall.Settimeofday(&syscall.Timeval{Sec: tv.Sec, Usec: 1 << 32}); err != syscall.EINVAL {
		t.Errorf("Settimeofday with Usec 1<<32 = %v; want EINVAL", err)
	}

	ts := syscall.NsecToTimespec(int64(150 * time.Millisecond))
	start := time.Now()
	if err := syscall.Nanosleep(&ts, nil); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Errorf("Nanosleep(150ms) returned after %v", d)
	}

	var tx syscall.Timex
	if _, err := syscall.Adjtimex(&tx); err != nil {
		t.Fatal(err)
	}
	if tx.Tick <= 0 {
		t.Errorf("Adjtimex Tick = %d", tx.Tick)
	}
	near(t, "Adjtimex Time", time.Unix(tx.Time.Sec, 0), now)

	// The test process has already used CPU time in the tests above.
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		t.Fatal(err)
	}
	cpu := time.Duration(syscall.TimevalToNsec(ru.Utime) + syscall.TimevalToNsec(ru.Stime))
	if ru.Utime.Usec < 0 || ru.Utime.Usec >= 1e6 || ru.Stime.Usec < 0 || ru.Stime.Usec >= 1e6 || cpu <= 0 || cpu > time.Hour {
		t.Errorf("Getrusage = %+v", ru)
	}
}

func TestSyscallABICredentials(t *testing.T) {
	// Without cgo, credential changes must reach every thread through
	// AllThreadsSyscall, which linux/wasm does not support yet. They must
	// fail cleanly rather than leave the module referring to a missing
	// runtime function.
	if err := syscall.Setuid(0); err != syscall.ENOTSUP {
		t.Errorf("Setuid(0) = %v; want ENOTSUP", err)
	}
	if _, _, err := syscall.AllThreadsSyscall(syscall.SYS_GETPID, 0, 0, 0); err != syscall.ENOTSUP {
		t.Errorf("AllThreadsSyscall = %v; want ENOTSUP", err)
	}
}

func TestSyscallABISocketTimeout(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fds[0])
	defer syscall.Close(fds[1])
	s := fds[0]
	tv := syscall.NsecToTimeval(int64(150 * time.Millisecond))
	if err := syscall.SetsockoptTimeval(s, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, _, err := syscall.Recvfrom(s, make([]byte, 8), 0)
		done <- err
	}()
	select {
	case err := <-done:
		if d := time.Since(start); err != syscall.EAGAIN || d < 100*time.Millisecond {
			t.Errorf("Recvfrom with a 150ms SO_RCVTIMEO = %v after %v; want EAGAIN", err, d)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Recvfrom with a 150ms SO_RCVTIMEO still blocked after 10s")
	}
}
