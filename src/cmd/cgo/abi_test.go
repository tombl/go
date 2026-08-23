// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"
)

func TestMixedPointerWidthProlog(t *testing.T) {
	p := &Package{PtrSize: 8, IntSize: 8, CPtrSize: 4}
	prolog := p.builtinProlog()
	if strings.Contains(prolog, "GOINTBITS") || strings.Contains(prolog, "CGOPTRBITS") {
		t.Fatal("cgo ABI placeholders remain in expanded prolog")
	}
	if !strings.Contains(prolog, "#if 32 != 64") {
		t.Fatal("expanded prolog does not select the mixed-pointer-width ABI")
	}
}
