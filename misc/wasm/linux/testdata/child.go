// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"syscall"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "exec" {
		if err := syscall.Exec("/grandchild", []string{"grandchild"}, os.Environ()); err != nil {
			panic(err)
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	fmt.Printf("child:%s cwd=%s\n", os.Getenv("GO_WASM_CHILD"), dir)
}
