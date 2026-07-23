// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux

#include "textflag.h"

TEXT runtime·exitThread(SB), NOSPLIT, $0-0
	UNDEF

TEXT runtime·osyield(SB), NOSPLIT, $0-0
	UNDEF
