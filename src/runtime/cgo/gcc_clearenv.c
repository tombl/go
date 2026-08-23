// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

#include "libcgo.h"

#include <stdlib.h>

/* Stub for calling clearenv */
CGO_ASMCGOCALL_RETURN_TYPE
x_cgo_clearenv(void **env __attribute__((unused)))
{
	_cgo_tsan_acquire();
	clearenv();
	_cgo_tsan_release();
	CGO_ASMCGOCALL_RETURN;
}
