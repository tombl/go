// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fpcell takes the address of a C function that package main also
// takes, so the program links two cgo packages' function-pointer cells for the
// same C symbol.
package fpcell

// #include <stdlib.h>
import "C"

import "unsafe"

// Free returns C's free as a C function pointer.
func Free() unsafe.Pointer {
	return unsafe.Pointer(C.free)
}
