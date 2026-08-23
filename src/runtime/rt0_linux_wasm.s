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

// __main_argc_argv_envp is the native wasm32 entry point expected by musl's
// crt startup. musl has already initialized libc, TLS, and constructors before
// reaching this adapter. sysargs performs the wasm32-to-Go pointer-vector
// conversion, so the native argc/argv/envp parameters are intentionally not
// copied into the 64-bit Go ABI frame here.
TEXT __main_argc_argv_envp(SB),NOSPLIT,$0
	MOVD $runtime·wasmStack+(m0Stack__size-16)(SB), SP

	// Preserve musl's wasm32 argc/argv in the ordinary Go startup frame.
	// sysargs recognizes cgo startup and reads the 32-bit vector without
	// asking the kernel to provide the process-argument blob a second time.
	Get SP
	Get R0
	I64ExtendI32U
	I64Store $0
	Get SP
	Get R1
	I64ExtendI32U
	I64Store $8

	I32Const $0 // entry PC_B
	Call runtime·rt0_go(SB)
	Drop
	Call wasm_pc_f_loop(SB)

	// The Go scheduler does not return, but musl's main hook has an int result.
	I32Const $0
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

// wasmSigtramp is called synchronously by the kernel while a syscall import is
// suspended. The instance globals still contain the interrupted M's SP and g.
// Its WebAssembly parameters are R0 (the registered handler) and R1 (signal).
TEXT runtime·wasmSigtramp(SB),NOSPLIT,$0
	Get SP
	I64ExtendI32U
	Set R2
	Get g
	Set R3

	MOVD g_m(R3), R4
	MOVD m_gsignal(R4), R5
	MOVD (g_stack+stack_hi)(R5), R6
	Get R6
	I64Const $16
	I64Sub
	I32WrapI64
	Set SP
	Get R5
	Set g

	Get SP
	I32Const $16
	I32Sub
	Set SP

	Get SP
	I64Const $0 // top-level return PC
	I64Store $0
	Get SP
	Get R1
	I64ExtendI32U
	I64Store $8

	I32Const $0
	Call runtime·wasmSignalHandler(SB)
	Drop

	Get R2
	I32WrapI64
	Set SP
	Get R3
	Set g
	Return
