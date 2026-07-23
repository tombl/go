// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

// wasmExecChild is the entrypoint for callback-based clone. It runs in a new
// WebAssembly instance with uninitialized globals, so establish a private
// stack before entering the nosplit child path.
TEXT ·wasmExecChild(SB),NOSPLIT,$0
	Get R0
	I64ExtendI32U
	Set R1
	Get R1
	I32WrapI64
	I64Load $0
	I32WrapI64
	Set SP

	Get SP
	I32Const $16
	I32Sub
	Set SP
	Get SP
	I64Const $0
	I64Store $0
	Get SP
	Get R1
	I64Store $8

	I32Const $0
	Call ·wasmExecChildGo(SB)
	Drop
	Return
