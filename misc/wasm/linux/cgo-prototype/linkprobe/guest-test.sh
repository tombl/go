#!/bin/busybox sh

fail() {
	printf 'cgo native link probe failure: %s\n' "$*"
	echo "::vm-test::fail"
	while :; do :; done
}

/bin/native-cgo-probe || fail "musl startup or exit failed"

echo "::vm-test::pass"
while :; do :; done
