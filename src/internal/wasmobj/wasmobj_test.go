// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wasmobj

import (
	"bytes"
	"testing"
)

func TestCustomSections(t *testing.T) {
	b := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	b = append(b, 0, 6, 4, 'n', 'a', 'm', 'e', 1)
	b = append(b, 1, 1, 0)
	f, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.Custom("name"), []byte{1}; !bytes.Equal(got, want) {
		t.Fatalf("Custom(name) = %v, want %v", got, want)
	}
	if got := len(f.Sections); got != 2 {
		t.Fatalf("len(Sections) = %d, want 2", got)
	}
}

func TestRejectsMalformedSection(t *testing.T) {
	b := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 1, 2, 0}
	if _, err := New(b); err == nil {
		t.Fatal("New accepted a truncated section")
	}
}

func TestFunctionImports(t *testing.T) {
	b := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	b = append(b, SectionType, 6, 1, 0x60, 1, 0x7f, 1, 0x7f)
	b = append(b, SectionImport, 17, 1, 5, 'l', 'i', 'n', 'u', 'x', 7, 's', 'y', 's', 'c', 'a', 'l', 'l', 0, 0)
	f, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	o, err := f.Object()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(o.Imports); got != 1 {
		t.Fatalf("len(Imports) = %d, want 1", got)
	}
	got := o.Imports[0]
	if got.Module != "linux" || got.Name != "syscall" || got.Index != 0 || got.TypeIndex != 0 {
		t.Fatalf("Imports[0] = %+v", got)
	}
}

func TestInitFunctions(t *testing.T) {
	o := new(Object)
	// linking version 2, INIT_FUNCS subsection containing two
	// (priority, symbol-index) pairs.
	if err := parseLinking([]byte{2, 6, 5, 2, 3, 9, 7, 11}, o); err != nil {
		t.Fatal(err)
	}
	want := []InitFunction{{Priority: 3, Symbol: 9}, {Priority: 7, Symbol: 11}}
	if len(o.InitFunctions) != len(want) {
		t.Fatalf("InitFunctions = %+v, want %+v", o.InitFunctions, want)
	}
	for i := range want {
		if o.InitFunctions[i] != want[i] {
			t.Fatalf("InitFunctions[%d] = %+v, want %+v", i, o.InitFunctions[i], want[i])
		}
	}
}

func TestTLSRelocationAddend(t *testing.T) {
	// DATA section 8, one TLS relocation at offset 5 against symbol 3,
	// with the signed addend -4.
	rels, err := parseRelocations([]byte{8, 1, RMemoryAddrTLSSLEB, 5, 3, 0x7c})
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || rels[0].Addend != -4 {
		t.Fatalf("relocations = %+v, want addend -4", rels)
	}
}
