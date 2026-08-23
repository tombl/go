// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

/*
#include <stdint.h>

uint32_t prototype_add32(uint32_t a, uint32_t b);
*/
import "C"

func main() {
	if got := uint32(C.prototype_add32(20, 22)); got != 42 {
		panic(got)
	}
}
