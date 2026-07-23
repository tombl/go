#!/usr/bin/env bash
# Copyright 2026 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
go_root=$(cd -- "$script_dir/../../.." && pwd)
distro_root=${WASM_LINUX_DISTRO:-"$go_root/../distro"}
go_binary=${GO_BINARY:-"$go_root/bin/go"}

if [[ ! -x $go_binary ]]; then
	echo "run.bash: build Go first; $go_binary does not exist" >&2
	exit 1
fi
if [[ ! -d $distro_root ]]; then
	echo "run.bash: set WASM_LINUX_DISTRO to the distro repository" >&2
	exit 1
fi

linux_module=${WASM_LINUX_MODULE:-}
if [[ -z $linux_module ]]; then
	linux_out=$(nix build --no-link --print-out-paths "$distro_root#linux")
	linux_module=$linux_out/dist/index.js
fi
runner=${WASM_LINUX_TEST_RUNNER:-"$distro_root/packages/vm-test/run-test.js"}

work_dir=$(mktemp -d)
trap 'rm -rf -- "$work_dir"' EXIT

(
	cd -- "$script_dir"
	GOOS=linux GOARCH=wasm CGO_ENABLED=0 \
		"$go_binary" test -c -o "$work_dir/init" .
	GOOS=linux GOARCH=wasm CGO_ENABLED=0 \
		"$go_binary" build -o "$work_dir/child" ./testdata/child.go
	GOOS=linux GOARCH=wasm CGO_ENABLED=0 \
		"$go_binary" build -o "$work_dir/grandchild" ./testdata/grandchild.go
)
if command -v wasm-validate >/dev/null; then
	wasm-validate --enable-threads "$work_dir/init"
fi
(
	cd -- "$work_dir"
	printf 'init\nchild\ngrandchild\n' | cpio --quiet -o -H newc >initramfs.cpio
)

timeout --kill-after=5 300 node "$runner" \
	--cpus 2 \
	"$linux_module" \
	"$work_dir/initramfs.cpio"
