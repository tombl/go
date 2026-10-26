// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

#include "libcgo.h"

#include <stdlib.h>

#ifdef __wasm__
/* Go passes an array of its 64-bit pointers on linux/wasm, while C pointers
 * are 32 bits. */
typedef uint64 cgo_env_arg;
#define CGO_ENV_ARG(arg, i) ((char *)(uintptr)(arg)[i])
#else
typedef char *cgo_env_arg;
#define CGO_ENV_ARG(arg, i) ((arg)[i])
#endif

/* Stub for calling setenv */
CGO_ASMCGOCALL_RETURN_TYPE
x_cgo_setenv(cgo_env_arg *arg)
{
	_cgo_tsan_acquire();
	setenv(CGO_ENV_ARG(arg, 0), CGO_ENV_ARG(arg, 1), 1);
	_cgo_tsan_release();
	CGO_ASMCGOCALL_RETURN;
}

/* Stub for calling unsetenv */
CGO_ASMCGOCALL_RETURN_TYPE
x_cgo_unsetenv(cgo_env_arg *arg)
{
	_cgo_tsan_acquire();
	unsetenv(CGO_ENV_ARG(arg, 0));
	_cgo_tsan_release();
	CGO_ASMCGOCALL_RETURN;
}
