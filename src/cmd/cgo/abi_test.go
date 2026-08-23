// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMixedPointerWidthProlog(t *testing.T) {
	p := &Package{PtrSize: 8, IntSize: 8, CPtrSize: 4}
	prolog := p.builtinProlog()
	if strings.Contains(prolog, "GOINTBITS") || strings.Contains(prolog, "CGOPTRBITS") {
		t.Fatal("cgo ABI placeholders remain in expanded prolog")
	}
	if !strings.Contains(prolog, "#if 32 != 64") {
		t.Fatal("expanded prolog does not select the mixed-pointer-width ABI")
	}
}

func TestWasmWrapperSignature(t *testing.T) {
	oldGoarch := goarch
	goarch = "wasm"
	t.Cleanup(func() { goarch = oldGoarch })

	p := &Package{PtrSize: 8, IntSize: 8, CPtrSize: 4, Written: make(map[string]bool)}
	n := &Name{
		Go:       "wrapped",
		C:        "wrapped",
		Mangle:   "_Cfunc_wrapped",
		FuncType: &FuncType{},
	}
	path := filepath.Join(t.TempDir(), "wrapper.c")
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	p.writeOutputFunc(out, n)
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(contents)
	if !strings.Contains(generated, "int\n_cgo") || !strings.Contains(generated, "\treturn 0;\n") {
		t.Fatalf("wasm wrapper does not implement int(void*) ABI:\n%s", generated)
	}
}

func TestWasmMallocWrapperABI(t *testing.T) {
	oldGoarch := goarch
	goarch = "wasm"
	t.Cleanup(func() { goarch = oldGoarch })

	p := &Package{PtrSize: 8, IntSize: 8, CPtrSize: 4}
	generated := p.cMallocDefC()
	for _, want := range []string{
		"int _cgo",
		"uint64_t r1",
		"a->r1 = (uint64_t)(uintptr_t)ret;",
		"return 0;",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("wasm malloc wrapper is missing %q:\n%s", want, generated)
		}
	}
}

func TestMixedPointerWidthStructFrame(t *testing.T) {
	p := &Package{PtrSize: 8, IntSize: 8, CPtrSize: 4}
	ptr := &Type{Size: 8, Align: 8, C: &TypeRepr{Repr: "uint32_t*"}, CgoPointer: true}
	u32 := &Type{Size: 4, Align: 4, C: &TypeRepr{Repr: "uint32_t"}}
	aggregate := &Type{
		Size:  16,
		Align: 8,
		C:     &TypeRepr{Repr: "buffer"},
		CgoStruct: &CgoStruct{Fields: []CgoStructField{
			{CName: "data", GoName: "data", Type: ptr},
			{CName: "length", GoName: "length", Type: u32},
		}},
	}
	frame := p.cgoFrameType(aggregate)
	for _, want := range []string{"uint64_t data", "uint32_t length", "__pad12[4]"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("mixed aggregate frame is missing %q: %s", want, frame)
		}
	}
	var out bytes.Buffer
	p.writeCgoPointerChecks(&out, "src", aggregate)
	p.writeCgoStructToC(&out, "dst", "src", aggregate)
	generated := out.String()
	for _, want := range []string{
		"src.data > UINTPTR_MAX",
		"dst.data = (uint32_t*)(uintptr_t)src.data",
		"dst.length = src.length",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("mixed aggregate marshal is missing %q:\n%s", want, generated)
		}
	}
}

func TestMixedPointerWidthExportPointer(t *testing.T) {
	p := &Package{PtrSize: 8, IntSize: 8, CPtrSize: 4}
	typ := p.cgoType(&ast.StarExpr{X: ast.NewIdent("uint32")})
	if !typ.CgoPointer || typ.Size != 8 || p.cgoFrameType(typ) != "uint64_t" {
		t.Fatalf("export pointer type = %+v, frame type %q", typ, p.cgoFrameType(typ))
	}
	var out bytes.Buffer
	p.writeCgoValueFromC(&out, "frame.p0", "native", typ)
	p.writeCgoPointerChecks(&out, "frame.r0", typ)
	p.writeCgoValueToC(&out, "nativeResult", "frame.r0", typ)
	generated := out.String()
	for _, want := range []string{
		"frame.p0 = (uint64_t)(uintptr_t)native",
		"frame.r0 > UINTPTR_MAX",
		"nativeResult = (GoUint32*)(uintptr_t)frame.r0",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("mixed export bridge is missing %q:\n%s", want, generated)
		}
	}
}

func TestMixedPointerWidthFrame(t *testing.T) {
	p := &Package{PtrSize: 8, IntSize: 8, CPtrSize: 4}
	ptr := &Type{
		Size:       8,
		Align:      8,
		C:          &TypeRepr{Repr: "void*"},
		Go:         ast.NewIdent("unsafe.Pointer"),
		CgoPointer: true,
	}
	frame, size := p.structType(&Name{FuncType: &FuncType{Params: []*Type{ptr}, Result: ptr}})
	if size != 16 {
		t.Fatalf("mixed-width frame size = %d, want 16", size)
	}
	if got := strings.Count(frame, "uint64_t"); got != 2 {
		t.Fatalf("mixed-width frame = %q, want two uint64_t slots", frame)
	}
}
