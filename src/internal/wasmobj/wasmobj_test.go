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
