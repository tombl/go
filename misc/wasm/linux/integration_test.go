// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && wasm

package linuxwasmtest

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

var payload = []byte("linux wasm go\n")

func TestFileAndPipeIO(t *testing.T) {
	const path = "/go-linux-wasm-test"
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("ReadFile returned %q", got)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	written := make(chan error, 1)
	go func() {
		_, err := w.Write(payload)
		if closeErr := w.Close(); err == nil {
			err = closeErr
		}
		written <- err
	}()
	got, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("pipe returned %q", got)
	}
}

func TestNetpollAndSocketABI(t *testing.T) {
	listener, err := net.Listen("unix", "/go-linux-wasm-stream.sock")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_, err = io.Copy(conn, conn)
			if closeErr := conn.Close(); err == nil {
				err = closeErr
			}
		}
		accepted <- err
	}()

	conn, err := net.DialTimeout("unix", listener.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// net.Buffers uses writev, whose C ABI has 32-bit pointers even though
	// Go's wasm implementation represents pointers as 64-bit values.
	buffers := net.Buffers{[]byte("linux "), []byte("wasm go\n")}
	if n, err := buffers.WriteTo(conn); err != nil || n != int64(len(payload)) {
		t.Fatalf("Buffers.WriteTo = %d, %v", n, err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("stream socket returned %q", got)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-accepted; err != nil {
		t.Fatal(err)
	}

	server, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: "/go-linux-wasm-dgram.sock", Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: "/go-linux-wasm-client.sock", Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	if n, _, err := client.WriteMsgUnix(payload, nil, server.LocalAddr().(*net.UnixAddr)); err != nil || n != len(payload) {
		t.Fatalf("WriteMsgUnix = %d, %v", n, err)
	}
	got = make([]byte, len(payload))
	if n, _, _, _, err := server.ReadMsgUnix(got, nil); err != nil || n != len(payload) {
		t.Fatalf("ReadMsgUnix = %d, %v", n, err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("datagram socket returned %q", got)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeThreads(t *testing.T) {
	old := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(old)

	var started atomic.Uint32
	var release atomic.Uint32
	done := make(chan struct{})
	go func() {
		started.Store(1)
		for release.Load() == 0 {
		}
		close(done)
	}()
	for started.Load() == 0 {
		runtime.Gosched()
	}
	// The spinning goroutine cannot yield. Reaching this store proves that it
	// is running concurrently on a second kernel thread.
	release.Store(1)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("non-yielding goroutine did not finish")
	}

	var mu sync.Mutex
	var workers sync.WaitGroup
	counter := 0
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 1000 {
				mu.Lock()
				counter++
				mu.Unlock()
			}
		}()
	}
	workers.Wait()
	if counter != 8000 {
		t.Fatalf("counter = %d, want 8000", counter)
	}
	runtime.GC()
}

func TestSignalCallback(t *testing.T) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGUSR1)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-signals:
		if got != syscall.SIGUSR1 {
			t.Fatalf("received %v, want SIGUSR1", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("signal callback did not reach os/signal")
	}
	signal.Stop(signals)

	// Notify must remember an inherited ignored disposition and restore it
	// when the notification is stopped.
	signal.Ignore(syscall.SIGUSR2)
	signal.Notify(signals, syscall.SIGUSR2)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-signals:
		if got != syscall.SIGUSR2 {
			t.Fatalf("received %v, want SIGUSR2", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("signal callback did not replace SIG_IGN")
	}
	signal.Stop(signals)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	signal.Reset(syscall.SIGUSR2)
}

func TestExec(t *testing.T) {
	cmd := exec.Command("/child", "argument")
	cmd.Dir = "/"
	cmd.Env = []string{"GO_WASM_CHILD=ok"}
	cmd.Stdin = bytes.NewReader(nil)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "child:ok cwd=/\n"; string(output) != want {
		t.Fatalf("child output = %q, want %q", output, want)
	}
	cmd = exec.Command("/child", "exec")
	cmd.Env = []string{"GO_WASM_CHILD=replaced"}
	cmd.Stdin = bytes.NewReader(nil)
	output, err = cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "grandchild:replaced\n"; string(output) != want {
		t.Fatalf("grandchild output = %q, want %q", output, want)
	}

	missing := exec.Command("/does-not-exist")
	missing.Stdin = bytes.NewReader(nil)
	missing.Stdout = io.Discard
	missing.Stderr = io.Discard
	if err := missing.Run(); !os.IsNotExist(err) {
		t.Fatalf("missing executable returned %v", err)
	}

	_, err = syscall.ForkExec("/child", []string{"/child"}, &syscall.ProcAttr{
		Sys: &syscall.SysProcAttr{Setsid: true},
	})
	if err == nil || !errors.Is(err, syscall.EOPNOTSUPP) {
		t.Fatalf("unsupported SysProcAttr returned %v", err)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		println("::vm-test::pass")
	} else {
		println("::vm-test::fail")
	}
	os.Exit(code)
}
