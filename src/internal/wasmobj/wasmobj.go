// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package wasmobj reads the section envelope shared by WebAssembly modules
// and relocatable WebAssembly object files.
package wasmobj

import (
	"bytes"
	"debug/dwarf"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

var (
	wasmMagic   = [4]byte{0x00, 0x61, 0x73, 0x6d}
	wasmVersion = [4]byte{0x01, 0x00, 0x00, 0x00}
)

// A File is a decoded WebAssembly section envelope. The payload of a custom
// section excludes its encoded name.
type File struct {
	Sections []Section
}

// A Section is one WebAssembly section.
type Section struct {
	ID   byte
	Name string
	Data []byte
}

// Open reads a WebAssembly module or relocatable object from name.
func Open(name string) (*File, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return New(b)
}

// New decodes the section envelope in b.
func New(b []byte) (*File, error) {
	if len(b) < 8 || !bytes.Equal(b[:4], wasmMagic[:]) {
		return nil, errors.New("not a WebAssembly file")
	}
	if !bytes.Equal(b[4:8], wasmVersion[:]) {
		return nil, fmt.Errorf("unsupported WebAssembly version %v", b[4:8])
	}

	f := new(File)
	for off := 8; off < len(b); {
		id := b[off]
		off++
		size, n, err := readULEB(b[off:])
		if err != nil {
			return nil, fmt.Errorf("section size: %w", err)
		}
		off += n
		if size > uint64(len(b)-off) {
			return nil, errors.New("WebAssembly section extends past end of file")
		}
		payload := b[off : off+int(size)]
		off += int(size)

		section := Section{ID: id, Data: payload}
		if id == 0 {
			nameLen, n, err := readULEB(payload)
			if err != nil {
				return nil, fmt.Errorf("custom section name: %w", err)
			}
			if nameLen > uint64(len(payload)-n) {
				return nil, errors.New("custom section name extends past section")
			}
			section.Name = string(payload[n : n+int(nameLen)])
			section.Data = payload[n+int(nameLen):]
		}
		f.Sections = append(f.Sections, section)
	}
	return f, nil
}

// Custom returns the contents of the first custom section named name.
func (f *File) Custom(name string) []byte {
	for _, section := range f.Sections {
		if section.ID == 0 && section.Name == name {
			return section.Data
		}
	}
	return nil
}

// DWARF returns the DWARF data carried in WebAssembly custom sections.
// LLVM writes explicit relocation addends into the debug section bytes, so
// type information is readable before the relocatable object is linked.
func (f *File) DWARF() (*dwarf.Data, error) {
	d, err := dwarf.New(
		f.Custom(".debug_abbrev"),
		f.Custom(".debug_aranges"),
		f.Custom(".debug_frame"),
		f.Custom(".debug_info"),
		f.Custom(".debug_line"),
		f.Custom(".debug_pubnames"),
		f.Custom(".debug_ranges"),
		f.Custom(".debug_str"),
	)
	if err != nil {
		return nil, err
	}
	if types := f.Custom(".debug_types"); len(types) != 0 {
		if err := d.AddTypes(".debug_types", types); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func readULEB(b []byte) (uint64, int, error) {
	var value uint64
	for i, c := range b {
		if i == binary.MaxVarintLen64 || i == binary.MaxVarintLen64-1 && c > 1 {
			return 0, 0, errors.New("ULEB128 value overflows uint64")
		}
		value |= uint64(c&0x7f) << (7 * i)
		if c&0x80 == 0 {
			return value, i + 1, nil
		}
	}
	return 0, 0, errors.New("truncated ULEB128 value")
}
