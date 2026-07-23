// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package atomic

import "unsafe"

//go:noescape
func Load(ptr *uint32) uint32

//go:noescape
func Load8(ptr *uint8) uint8

//go:noescape
func Load64(ptr *uint64) uint64

//go:noescape
func Xadd(ptr *uint32, delta int32) uint32

//go:noescape
func Xadd64(ptr *uint64, delta int64) uint64

//go:noescape
func Xchg(ptr *uint32, new uint32) uint32

//go:noescape
func Xchg8(ptr *uint8, new uint8) uint8

//go:noescape
func Xchg64(ptr *uint64, new uint64) uint64

//go:noescape
func And8(ptr *uint8, val uint8)

//go:noescape
func Or8(ptr *uint8, val uint8)

//go:noescape
func And(ptr *uint32, val uint32)

//go:noescape
func Or(ptr *uint32, val uint32)

//go:noescape
func Cas(ptr *uint32, old, new uint32) bool

//go:noescape
func Cas64(ptr *uint64, old, new uint64) bool

//go:noescape
func Store(ptr *uint32, val uint32)

//go:noescape
func Store8(ptr *uint8, val uint8)

//go:noescape
func Store64(ptr *uint64, val uint64)

// StorepNoWB performs *ptr = val atomically and without a write barrier.
// NO go:noescape annotation; see atomic_pointer.go.
func StorepNoWB(ptr unsafe.Pointer, val unsafe.Pointer)

//go:nosplit
func Loadp(ptr unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(uintptr(Load64((*uint64)(ptr))))
}

//go:nosplit
func LoadAcq(ptr *uint32) uint32 { return Load(ptr) }

//go:nosplit
func LoadAcq64(ptr *uint64) uint64 { return Load64(ptr) }

//go:nosplit
func LoadAcquintptr(ptr *uintptr) uintptr {
	return uintptr(Load64((*uint64)(unsafe.Pointer(ptr))))
}

//go:nosplit
func Xadduintptr(ptr *uintptr, delta uintptr) uintptr {
	return uintptr(Xadd64((*uint64)(unsafe.Pointer(ptr)), int64(delta)))
}

//go:nosplit
func Xchgint32(ptr *int32, new int32) int32 {
	return int32(Xchg((*uint32)(unsafe.Pointer(ptr)), uint32(new)))
}

//go:nosplit
func Xchgint64(ptr *int64, new int64) int64 {
	return int64(Xchg64((*uint64)(unsafe.Pointer(ptr)), uint64(new)))
}

//go:nosplit
func Xchguintptr(ptr *uintptr, new uintptr) uintptr {
	return uintptr(Xchg64((*uint64)(unsafe.Pointer(ptr)), uint64(new)))
}

//go:nosplit
func Casint32(ptr *int32, old, new int32) bool {
	return Cas((*uint32)(unsafe.Pointer(ptr)), uint32(old), uint32(new))
}

//go:nosplit
func Casint64(ptr *int64, old, new int64) bool {
	return Cas64((*uint64)(unsafe.Pointer(ptr)), uint64(old), uint64(new))
}

//go:nosplit
func Casp1(ptr *unsafe.Pointer, old, new unsafe.Pointer) bool {
	return Cas64((*uint64)(unsafe.Pointer(ptr)), uint64(uintptr(old)), uint64(uintptr(new)))
}

//go:nosplit
func Casuintptr(ptr *uintptr, old, new uintptr) bool {
	return Cas64((*uint64)(unsafe.Pointer(ptr)), uint64(old), uint64(new))
}

//go:nosplit
func CasRel(ptr *uint32, old, new uint32) bool { return Cas(ptr, old, new) }

//go:nosplit
func StoreRel(ptr *uint32, val uint32) { Store(ptr, val) }

//go:nosplit
func StoreRel64(ptr *uint64, val uint64) { Store64(ptr, val) }

//go:nosplit
func StoreReluintptr(ptr *uintptr, val uintptr) {
	Store64((*uint64)(unsafe.Pointer(ptr)), uint64(val))
}

//go:nosplit
func Storeint32(ptr *int32, val int32) {
	Store((*uint32)(unsafe.Pointer(ptr)), uint32(val))
}

//go:nosplit
func Storeint64(ptr *int64, val int64) {
	Store64((*uint64)(unsafe.Pointer(ptr)), uint64(val))
}

//go:nosplit
func Storeuintptr(ptr *uintptr, val uintptr) {
	Store64((*uint64)(unsafe.Pointer(ptr)), uint64(val))
}

//go:nosplit
func Loaduintptr(ptr *uintptr) uintptr {
	return uintptr(Load64((*uint64)(unsafe.Pointer(ptr))))
}

//go:nosplit
func Loaduint(ptr *uint) uint {
	return uint(Load64((*uint64)(unsafe.Pointer(ptr))))
}

//go:nosplit
func Loadint32(ptr *int32) int32 {
	return int32(Load((*uint32)(unsafe.Pointer(ptr))))
}

//go:nosplit
func Loadint64(ptr *int64) int64 {
	return int64(Load64((*uint64)(unsafe.Pointer(ptr))))
}

//go:nosplit
func Xaddint32(ptr *int32, delta int32) int32 {
	return int32(Xadd((*uint32)(unsafe.Pointer(ptr)), delta))
}

//go:nosplit
func Xaddint64(ptr *int64, delta int64) int64 {
	return int64(Xadd64((*uint64)(unsafe.Pointer(ptr)), delta))
}
