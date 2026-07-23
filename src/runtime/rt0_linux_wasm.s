// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "go_asm.h"
#include "textflag.h"

TEXT _rt0_wasm_linux(SB),NOSPLIT,$0
	MOVD $runtime·wasmStack+(m0Stack__size-16)(SB), SP

	I32Const $0 // entry PC_B
	Call runtime·rt0_go(SB)
	Drop
	Call wasm_pc_f_loop(SB)

	Return

// wasmMstart is called by the kernel's callback-based clone syscall. Each
// worker has its own WebAssembly globals, so initialize SP and g from mp
// before entering the ordinary Go M startup path.
TEXT runtime·wasmMstart(SB),NOSPLIT,$0
	Get R0
	I64ExtendI32U
	Set R1
	MOVD m_g0(R1), R2
	MOVD (g_stack+stack_hi)(R2), R3
	Get R3
	I64Const $16
	I64Sub
	I32WrapI64
	Set SP
	Get R2
	Set g

	I32Const $0
	Call runtime·mstart(SB)
	Drop
	Call wasm_pc_f_loop(SB)
	Return
