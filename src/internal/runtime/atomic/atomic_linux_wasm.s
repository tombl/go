// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

TEXT ·Load(SB), NOSPLIT, $0-12
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I32LoadAtomic $0
	I32Store ret+8(FP)
	RET

TEXT ·Load8(SB), NOSPLIT, $0-9
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I32LoadAtomic8U $0
	I32Store8 ret+8(FP)
	RET

TEXT ·Load64(SB), NOSPLIT, $0-16
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I64LoadAtomic $0
	I64Store ret+8(FP)
	RET

TEXT ·Xadd(SB), NOSPLIT, $0-20
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load delta+8(FP)
	I32AddRmw $0
	I32Load delta+8(FP)
	I32Add
	I32Store ret+16(FP)
	RET

TEXT ·Xadd64(SB), NOSPLIT, $0-24
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I64Load delta+8(FP)
	I64AddRmw $0
	I64Load delta+8(FP)
	I64Add
	I64Store ret+16(FP)
	RET

TEXT ·Xchg(SB), NOSPLIT, $0-20
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load new+8(FP)
	I32Xchg $0
	I32Store ret+16(FP)
	RET

TEXT ·Xchg8(SB), NOSPLIT, $0-17
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load8U new+8(FP)
	I32Xchg8U $0
	I32Store8 ret+16(FP)
	RET

TEXT ·Xchg64(SB), NOSPLIT, $0-24
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I64Load new+8(FP)
	I64Xchg $0
	I64Store ret+16(FP)
	RET

TEXT ·And8(SB), NOSPLIT, $0-9
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load8U val+8(FP)
	I32AndRmw8U $0
	Drop
	RET

TEXT ·Or8(SB), NOSPLIT, $0-9
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load8U val+8(FP)
	I32OrRmw8U $0
	Drop
	RET

TEXT ·And(SB), NOSPLIT, $0-12
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load val+8(FP)
	I32AndRmw $0
	Drop
	RET

TEXT ·Or(SB), NOSPLIT, $0-12
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load val+8(FP)
	I32OrRmw $0
	Drop
	RET

TEXT ·Cas(SB), NOSPLIT, $0-17
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load old+8(FP)
	I32Load new+12(FP)
	I32Cmpxchg $0
	I32Load old+8(FP)
	I32Eq
	I32Store8 ret+16(FP)
	RET

TEXT ·Cas64(SB), NOSPLIT, $0-25
	Get SP
	I64Load ptr+0(FP)
	I32WrapI64
	I64Load old+8(FP)
	I64Load new+16(FP)
	I64Cmpxchg $0
	I64Load old+8(FP)
	I64Eq
	I32Store8 ret+24(FP)
	RET

TEXT ·Store(SB), NOSPLIT, $0-12
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load val+8(FP)
	I32StoreAtomic $0
	RET

TEXT ·Store8(SB), NOSPLIT, $0-9
	I64Load ptr+0(FP)
	I32WrapI64
	I32Load8U val+8(FP)
	I32StoreAtomic8 $0
	RET

TEXT ·Store64(SB), NOSPLIT, $0-16
	I64Load ptr+0(FP)
	I32WrapI64
	I64Load val+8(FP)
	I64StoreAtomic $0
	RET

TEXT ·StorepNoWB(SB), NOSPLIT, $0-16
	I64Load ptr+0(FP)
	I32WrapI64
	I64Load val+8(FP)
	I64StoreAtomic $0
	RET
