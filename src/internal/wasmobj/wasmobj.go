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
	Index int
	ID    byte
	Name  string
	Data  []byte
}

// The standard section IDs used by relocatable WebAssembly objects.
const (
	SectionCustom   = 0
	SectionType     = 1
	SectionImport   = 2
	SectionFunction = 3
	SectionCode     = 10
	SectionData     = 11
)

// A FuncType is a WebAssembly function signature.
type FuncType struct {
	Params  []byte
	Results []byte
}

// A Function is a function definition in a relocatable object. Body begins
// with the local declarations and does not include the encoded body size.
// CodeOffset is the offset of Body in the relocation coordinate system for
// the CODE section (whose origin follows the function-count field).
type Function struct {
	Index      uint32
	TypeIndex  uint32
	Body       []byte
	CodeOffset uint64
}

// A DataSegment is one data segment in a relocatable object.
type DataSegment struct {
	Name  string
	Align uint32 // log2 alignment, as encoded by the linking section
	Flags uint32
	Data  []byte
}

// WebAssembly linking symbol kinds.
const (
	SymbolFunction = iota
	SymbolData
	SymbolGlobal
	SymbolSection
	SymbolTag
	SymbolTable
)

// WebAssembly linking symbol flags.
const (
	SymBindingWeak    = 0x1
	SymBindingLocal   = 0x2
	SymVisibilityHide = 0x4
	SymUndefined      = 0x10
	SymExplicitName   = 0x40
)

// A Symbol is an entry in the linking section's symbol table.
type Symbol struct {
	Kind  uint8
	Flags uint32
	Name  string
	Index uint32

	Segment uint32
	Offset  uint64
	Size    uint64
}

// Relocation types from the WebAssembly tool-conventions linking ABI.
const (
	RFunctionIndexLEB = iota
	RTableIndexSLEB
	RTableIndexI32
	RMemoryAddrLEB
	RMemoryAddrSLEB
	RMemoryAddrI32
	RTypeIndexLEB
	RGlobalIndexLEB
	RFunctionOffsetI32
	RSectionOffsetI32
	RTagIndexLEB
	RMemoryAddrRelSLEB
	RTableIndexRelSLEB
	RGlobalIndexI32
	RMemoryAddrLEB64
	RMemoryAddrSLEB64
	RMemoryAddrI64
	RMemoryAddrRelSLEB64
	RTableIndexSLEB64
	RTableIndexI64
	RTableNumberLEB
)

// A Relocation is an entry in a reloc.* custom section. Section is the
// zero-based index of the section to which Offset applies. Index addresses the
// linking symbol table, except for RTypeIndexLEB where it addresses Types.
type Relocation struct {
	Type    uint32
	Section uint32
	Offset  uint64
	Index   uint32
	Addend  int64
}

// An Object is the relocatable-object information needed by a linker.
type Object struct {
	Types       []FuncType
	Functions   []Function
	Segments    []DataSegment
	Symbols     []Symbol
	Relocations []Relocation
	CodeSection uint32
	DataSection uint32
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

		section := Section{Index: len(f.Sections), ID: id, Data: payload}
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

// Object decodes the relocatable-object sections in f. It rejects constructs
// it cannot account for instead of silently producing a partially linked
// object.
func (f *File) Object() (*Object, error) {
	o := new(Object)
	var funcImports uint32
	var funcTypes []uint32
	var imports importNames
	var code *Section

	for i := range f.Sections {
		s := &f.Sections[i]
		var err error
		switch s.ID {
		case SectionType:
			o.Types, err = parseTypes(s.Data)
		case SectionImport:
			funcImports, funcTypes, imports, err = parseImports(s.Data)
		case SectionFunction:
			var defs []uint32
			defs, err = parseU32Vector(s.Data)
			funcTypes = append(funcTypes, defs...)
		case SectionCode:
			code = s
			o.CodeSection = uint32(s.Index)
		case SectionData:
			o.DataSection = uint32(s.Index)
		}
		if err != nil {
			return nil, fmt.Errorf("WebAssembly section %d: %w", s.Index, err)
		}
	}
	if code != nil {
		var err error
		o.Functions, err = parseCode(code.Data, funcImports, funcTypes)
		if err != nil {
			return nil, fmt.Errorf("WebAssembly CODE section: %w", err)
		}
	}
	if linking := f.Custom("linking"); linking != nil {
		if err := parseLinking(linking, o); err != nil {
			return nil, fmt.Errorf("WebAssembly linking section: %w", err)
		}
		for i := range o.Symbols {
			s := &o.Symbols[i]
			if s.Name != "" || s.Flags&SymUndefined == 0 {
				continue
			}
			switch s.Kind {
			case SymbolFunction:
				s.Name = imports.function[s.Index]
			case SymbolGlobal:
				s.Name = imports.global[s.Index]
			case SymbolTag:
				s.Name = imports.tag[s.Index]
			case SymbolTable:
				s.Name = imports.table[s.Index]
			}
		}
	}
	for _, s := range f.Sections {
		if s.ID == SectionCustom && len(s.Name) > len("reloc.") && s.Name[:len("reloc.")] == "reloc." {
			rels, err := parseRelocations(s.Data)
			if err != nil {
				return nil, fmt.Errorf("WebAssembly %s section: %w", s.Name, err)
			}
			o.Relocations = append(o.Relocations, rels...)
		}
	}
	if err := parseData(f, o); err != nil {
		return nil, fmt.Errorf("WebAssembly DATA section: %w", err)
	}
	return o, nil
}

type decoder struct {
	b   []byte
	off int
}

func (d *decoder) byte() (byte, error) {
	if d.off >= len(d.b) {
		return 0, errors.New("truncated data")
	}
	v := d.b[d.off]
	d.off++
	return v, nil
}

func (d *decoder) uleb() (uint64, error) {
	v, n, err := readULEB(d.b[d.off:])
	if err == nil {
		d.off += n
	}
	return v, err
}

func (d *decoder) sleb() (int64, error) {
	var v int64
	var shift uint
	for {
		c, err := d.byte()
		if err != nil {
			return 0, err
		}
		v |= int64(c&0x7f) << shift
		shift += 7
		if c&0x80 == 0 {
			if shift < 64 && c&0x40 != 0 {
				v |= ^0 << shift
			}
			return v, nil
		}
		if shift >= 64 {
			return 0, errors.New("SLEB128 value overflows int64")
		}
	}
}

func (d *decoder) name() (string, error) {
	n, err := d.uleb()
	if err != nil || n > uint64(len(d.b)-d.off) {
		if err == nil {
			err = errors.New("name extends past data")
		}
		return "", err
	}
	s := string(d.b[d.off : d.off+int(n)])
	d.off += int(n)
	return s, nil
}

func (d *decoder) bytes(n uint64) ([]byte, error) {
	if n > uint64(len(d.b)-d.off) {
		return nil, errors.New("record extends past data")
	}
	b := d.b[d.off : d.off+int(n)]
	d.off += int(n)
	return b, nil
}

func parseTypes(b []byte) ([]FuncType, error) {
	d := decoder{b: b}
	n, err := d.uleb()
	if err != nil {
		return nil, err
	}
	types := make([]FuncType, 0, n)
	for range n {
		form, err := d.byte()
		if err != nil || form != 0x60 {
			return nil, fmt.Errorf("invalid function type form %#x", form)
		}
		params, err := parseValueTypes(&d)
		if err != nil {
			return nil, err
		}
		results, err := parseValueTypes(&d)
		if err != nil {
			return nil, err
		}
		types = append(types, FuncType{Params: params, Results: results})
	}
	return types, nil
}

func parseValueTypes(d *decoder) ([]byte, error) {
	n, err := d.uleb()
	if err != nil {
		return nil, err
	}
	b, err := d.bytes(n)
	return append([]byte(nil), b...), err
}

func parseU32Vector(b []byte) ([]uint32, error) {
	d := decoder{b: b}
	n, err := d.uleb()
	if err != nil {
		return nil, err
	}
	v := make([]uint32, n)
	for i := range v {
		x, err := d.uleb()
		if err != nil || x > uint64(^uint32(0)) {
			return nil, errors.New("invalid uint32 vector element")
		}
		v[i] = uint32(x)
	}
	return v, nil
}

type importNames struct {
	function map[uint32]string
	global   map[uint32]string
	tag      map[uint32]string
	table    map[uint32]string
}

func parseImports(b []byte) (uint32, []uint32, importNames, error) {
	d := decoder{b: b}
	n, err := d.uleb()
	if err != nil {
		return 0, nil, importNames{}, err
	}
	names := importNames{
		function: make(map[uint32]string),
		global:   make(map[uint32]string),
		tag:      make(map[uint32]string),
		table:    make(map[uint32]string),
	}
	var funcs uint32
	var globals, tags, tables uint32
	var types []uint32
	for range n {
		if _, err := d.name(); err != nil {
			return 0, nil, importNames{}, err
		}
		name, err := d.name()
		if err != nil {
			return 0, nil, importNames{}, err
		}
		kind, err := d.byte()
		if err != nil {
			return 0, nil, importNames{}, err
		}
		switch kind {
		case 0: // function
			t, err := d.uleb()
			if err != nil {
				return 0, nil, importNames{}, err
			}
			names.function[funcs] = name
			funcs++
			types = append(types, uint32(t))
		case 1: // table
			names.table[tables] = name
			tables++
			if _, err := d.byte(); err != nil {
				return 0, nil, importNames{}, err
			}
			if err := skipLimits(&d); err != nil {
				return 0, nil, importNames{}, err
			}
		case 2: // memory
			if err := skipLimits(&d); err != nil {
				return 0, nil, importNames{}, err
			}
		case 3: // global
			names.global[globals] = name
			globals++
			if _, err := d.bytes(2); err != nil {
				return 0, nil, importNames{}, err
			}
		case 4: // tag
			names.tag[tags] = name
			tags++
			if _, err := d.byte(); err != nil {
				return 0, nil, importNames{}, err
			}
			if _, err := d.uleb(); err != nil {
				return 0, nil, importNames{}, err
			}
		default:
			return 0, nil, importNames{}, fmt.Errorf("unsupported import kind %d", kind)
		}
	}
	return funcs, types, names, nil
}

func skipLimits(d *decoder) error {
	flags, err := d.uleb()
	if err != nil {
		return err
	}
	if _, err := d.uleb(); err != nil {
		return err
	}
	if flags&1 != 0 {
		_, err = d.uleb()
	}
	return err
}

func parseCode(b []byte, imported uint32, types []uint32) ([]Function, error) {
	d := decoder{b: b}
	n, err := d.uleb()
	if err != nil {
		return nil, err
	}
	funcs := make([]Function, 0, n)
	for i := uint32(0); i < uint32(n); i++ {
		size, err := d.uleb()
		if err != nil {
			return nil, err
		}
		// Relocation offsets are relative to the beginning of the CODE
		// payload, including its function-count field.
		bodyOffset := uint64(d.off)
		body, err := d.bytes(size)
		if err != nil {
			return nil, err
		}
		idx := imported + i
		if int(idx) >= len(types) {
			return nil, errors.New("CODE and FUNCTION section lengths differ")
		}
		funcs = append(funcs, Function{Index: idx, TypeIndex: types[idx], Body: body, CodeOffset: bodyOffset})
	}
	return funcs, nil
}

func parseLinking(b []byte, o *Object) error {
	d := decoder{b: b}
	version, err := d.uleb()
	if err != nil || version != 2 {
		return fmt.Errorf("unsupported linking version %d", version)
	}
	for d.off < len(d.b) {
		id, err := d.byte()
		if err != nil {
			return err
		}
		size, err := d.uleb()
		if err != nil {
			return err
		}
		payload, err := d.bytes(size)
		if err != nil {
			return err
		}
		switch id {
		case 5:
			if err := parseSegmentInfo(payload, o); err != nil {
				return err
			}
		case 8:
			if err := parseSymbols(payload, o); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseSegmentInfo(b []byte, o *Object) error {
	d := decoder{b: b}
	n, err := d.uleb()
	if err != nil {
		return err
	}
	o.Segments = make([]DataSegment, n)
	for i := range o.Segments {
		name, err := d.name()
		if err != nil {
			return err
		}
		align, err := d.uleb()
		if err != nil {
			return err
		}
		flags, err := d.uleb()
		if err != nil {
			return err
		}
		o.Segments[i] = DataSegment{Name: name, Align: uint32(align), Flags: uint32(flags)}
	}
	return nil
}

func parseSymbols(b []byte, o *Object) error {
	d := decoder{b: b}
	n, err := d.uleb()
	if err != nil {
		return err
	}
	o.Symbols = make([]Symbol, 0, n)
	for range n {
		kind, err := d.byte()
		if err != nil {
			return err
		}
		flags, err := d.uleb()
		if err != nil {
			return err
		}
		s := Symbol{Kind: kind, Flags: uint32(flags)}
		switch kind {
		case SymbolFunction, SymbolGlobal, SymbolTag, SymbolTable:
			index, err := d.uleb()
			if err != nil {
				return err
			}
			s.Index = uint32(index)
			if flags&SymUndefined == 0 || flags&SymExplicitName != 0 {
				s.Name, err = d.name()
				if err != nil {
					return err
				}
			}
		case SymbolData:
			s.Name, err = d.name()
			if err != nil {
				return err
			}
			if flags&SymUndefined == 0 {
				seg, err := d.uleb()
				if err != nil {
					return err
				}
				s.Segment = uint32(seg)
				if s.Offset, err = d.uleb(); err != nil {
					return err
				}
				if s.Size, err = d.uleb(); err != nil {
					return err
				}
			}
		case SymbolSection:
			index, err := d.uleb()
			if err != nil {
				return err
			}
			s.Index = uint32(index)
		default:
			return fmt.Errorf("unsupported symbol kind %d", kind)
		}
		o.Symbols = append(o.Symbols, s)
	}
	return nil
}

func parseRelocations(b []byte) ([]Relocation, error) {
	d := decoder{b: b}
	section, err := d.uleb()
	if err != nil {
		return nil, err
	}
	n, err := d.uleb()
	if err != nil {
		return nil, err
	}
	rels := make([]Relocation, 0, n)
	for range n {
		typ, err := d.uleb()
		if err != nil {
			return nil, err
		}
		off, err := d.uleb()
		if err != nil {
			return nil, err
		}
		idx, err := d.uleb()
		if err != nil {
			return nil, err
		}
		r := Relocation{Type: uint32(typ), Section: uint32(section), Offset: off, Index: uint32(idx)}
		switch r.Type {
		case RMemoryAddrLEB, RMemoryAddrSLEB, RMemoryAddrI32,
			RFunctionOffsetI32, RSectionOffsetI32, RMemoryAddrRelSLEB,
			RMemoryAddrLEB64, RMemoryAddrSLEB64, RMemoryAddrI64,
			RMemoryAddrRelSLEB64, RTableIndexSLEB64, RTableIndexI64:
			r.Addend, err = d.sleb()
			if err != nil {
				return nil, err
			}
		}
		rels = append(rels, r)
	}
	return rels, nil
}

func parseData(f *File, o *Object) error {
	var data []byte
	for _, s := range f.Sections {
		if s.ID == SectionData {
			data = s.Data
			break
		}
	}
	if data == nil {
		return nil
	}
	d := decoder{b: data}
	n, err := d.uleb()
	if err != nil {
		return err
	}
	if len(o.Segments) == 0 {
		o.Segments = make([]DataSegment, n)
	} else if uint64(len(o.Segments)) != n {
		return errors.New("DATA and segment-info lengths differ")
	}
	for i := range o.Segments {
		flags, err := d.uleb()
		if err != nil {
			return err
		}
		if flags&2 != 0 {
			if _, err := d.uleb(); err != nil { // memory index
				return err
			}
		}
		if flags&1 == 0 {
			op, err := d.byte()
			if err != nil || op != 0x41 {
				return errors.New("unsupported data offset expression")
			}
			if _, err := d.sleb(); err != nil {
				return err
			}
			end, err := d.byte()
			if err != nil || end != 0x0b {
				return errors.New("unterminated data offset expression")
			}
		}
		size, err := d.uleb()
		if err != nil {
			return err
		}
		contents, err := d.bytes(size)
		if err != nil {
			return err
		}
		o.Segments[i].Data = contents
	}
	return nil
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
