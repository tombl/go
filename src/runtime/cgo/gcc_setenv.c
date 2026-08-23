// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

#include "libcgo.h"

#include <stdlib.h>

/* Stub for calling setenv */
CGO_ASMCGOCALL_RETURN_TYPE
x_cgo_setenv(char **arg)
{
	_cgo_tsan_acquire();
	setenv(arg[0], arg[1], 1);
	_cgo_tsan_release();
	CGO_ASMCGOCALL_RETURN;
}

/* Stub for calling unsetenv */
CGO_ASMCGOCALL_RETURN_TYPE
x_cgo_unsetenv(char **arg)
{
	_cgo_tsan_acquire();
	unsetenv(arg[0]);
	_cgo_tsan_release();
	CGO_ASMCGOCALL_RETURN;
}
