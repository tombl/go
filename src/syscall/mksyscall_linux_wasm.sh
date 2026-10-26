#!/bin/sh
# Copyright 2026 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# mksyscall_linux_wasm.sh generates zsyscall_linux_wasm.go.
#
# The wasm32 kernel takes i386-style 32-bit argument slots, so 64-bit
# arguments are split low word first (-l32). Go's time types on linux/wasm
# follow musl's time64 layout, while some shared wrappers in syscall_linux.go
# use time32 entries or the kernel's old-timeval structures. Those wrappers
# are reimplemented in syscall_linux_wasm.go and filtered out here.

set -e

overrides='Adjtimex|Getrusage|Nanosleep|Settimeofday|utimensat|wait4'

tags=
if [ "$1" = "-tags" ]; then
	tags="$2"
	shift 2
fi
linux=$1
arch=$2
filtered=$(mktemp -d)/$linux
trap 'rm -rf "$(dirname "$filtered")"' EXIT
grep -Ev "^//sys(nb)?[[:space:]]+($overrides)\(" "$linux" >"$filtered"
./mksyscall.pl -l32 -tags "$tags" "$filtered" "$arch" |
	sed "s|^// mksyscall.pl -l32 -tags $tags .*|// mksyscall_linux_wasm.sh -tags $tags $linux $arch|"
