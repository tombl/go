// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cgo

import _ "unsafe" // for go:linkname

//go:cgo_import_static x_cgo_wasm_alloc
//go:linkname x_cgo_wasm_alloc x_cgo_wasm_alloc
//go:linkname _cgo_wasm_alloc _cgo_wasm_alloc
var x_cgo_wasm_alloc byte
var _cgo_wasm_alloc = &x_cgo_wasm_alloc
