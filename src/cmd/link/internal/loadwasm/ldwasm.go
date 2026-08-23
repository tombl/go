// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package loadwasm loads relocatable WebAssembly host objects into the Go
// linker's symbol graph.
package loadwasm

import (
	"bytes"
	"cmd/internal/bio"
	"cmd/internal/obj"
	"cmd/internal/objabi"
	"cmd/internal/sys"
	"cmd/link/internal/loader"
	"cmd/link/internal/sym"
	"fmt"
	"internal/wasmobj"
)

// Load loads one relocatable WebAssembly object. The Go linker remains the
// final linker; this is intentionally not an external wasm-ld path.
func Load(l *loader.Loader, arch *sys.Arch, localSymVersion int, f *bio.Reader, pkg string, length int64, pn string) ([]loader.Sym, error) {
	if arch.Family != sys.Wasm {
		return nil, fmt.Errorf("loadwasm: %s: WebAssembly object for %s", pn, arch.Name)
	}
	b, _, err := f.Slice(uint64(length))
	if err != nil {
		return nil, fmt.Errorf("loadwasm: %s: %v", pn, err)
	}
	wf, err := wasmobj.New(b)
	if err != nil {
		return nil, fmt.Errorf("loadwasm: %s: %v", pn, err)
	}
	o, err := wf.Object()
	if err != nil {
		return nil, fmt.Errorf("loadwasm: %s: %v", pn, err)
	}

	typeSyms := make([]loader.Sym, len(o.Types))
	for i, typ := range o.Types {
		ft, err := goFuncType(typ)
		if err != nil {
			return nil, fmt.Errorf("loadwasm: %s: type %d: %v", pn, i, err)
		}
		var data bytes.Buffer
		ft.Write(&data)
		bld := l.MakeSymbolBuilder(fmt.Sprintf("wasm.type.%s.%d", pn, i))
		bld.SetType(sym.SRODATA)
		// The type is linker metadata, not program data. Keep the bytes in
		// Loader's retained wasm-type table without allocating linear memory.
		bld.SetSize(0)
		bld.SetReachable(true)
		typeSyms[i] = bld.Sym()
		l.SetWasmTypeData(typeSyms[i], data.Bytes())
	}

	syms := make([]loader.Sym, len(o.Symbols))
	symKinds := make(map[loader.Sym]uint8)
	for i, ws := range o.Symbols {
		if ws.Kind == wasmobj.SymbolSection {
			continue
		}
		if ws.Name == "" {
			return nil, fmt.Errorf("loadwasm: %s: symbol %d has no name", pn, i)
		}
		version := 0
		if ws.Flags&wasmobj.SymBindingLocal != 0 {
			version = localSymVersion
		}
		s := l.LookupOrCreateCgoExport(ws.Name, version)
		syms[i] = s
		symKinds[s] = ws.Kind
		if ws.Flags&wasmobj.SymBindingLocal != 0 {
			l.SetAttrLocal(s, true)
			l.SetAttrDuplicateOK(s, true)
		}
		if ws.Flags&wasmobj.SymBindingWeak != 0 {
			l.SetAttrDuplicateOK(s, true)
			l.SetSymWeakBinding(s, true)
		}
		if ws.Flags&wasmobj.SymVisibilityHide != 0 {
			l.SetAttrVisibilityHidden(s, true)
		}
	}

	functions := make(map[uint32]wasmobj.Function, len(o.Functions))
	for _, fn := range o.Functions {
		functions[fn.Index] = fn
	}
	var textp []loader.Sym
	for i, ws := range o.Symbols {
		if ws.Flags&wasmobj.SymUndefined != 0 || syms[i] == 0 {
			continue
		}
		bld := l.MakeSymbolUpdater(syms[i])
		switch ws.Kind {
		case wasmobj.SymbolFunction:
			fn, ok := functions[ws.Index]
			if !ok {
				return nil, fmt.Errorf("loadwasm: %s: function %s has no body", pn, ws.Name)
			}
			if int(fn.TypeIndex) >= len(typeSyms) {
				return nil, fmt.Errorf("loadwasm: %s: function %s has invalid type %d", pn, ws.Name, fn.TypeIndex)
			}
			if bld.Type() != sym.Sxxx && !bld.DuplicateOK() {
				return nil, fmt.Errorf("loadwasm: %s: duplicate symbol %s", pn, ws.Name)
			}
			bld.SetType(sym.STEXT)
			bld.SetData(append([]byte(nil), fn.Body...))
			bld.SetSize(int64(len(fn.Body)))
			l.SetWasmTypeSym(syms[i], typeSyms[fn.TypeIndex])
			bld.SetExternal(true)
			bld.SetOnList(true)
			textp = append(textp, syms[i])
		case wasmobj.SymbolData:
			if int(ws.Segment) >= len(o.Segments) {
				return nil, fmt.Errorf("loadwasm: %s: data symbol %s has invalid segment %d", pn, ws.Name, ws.Segment)
			}
			seg := o.Segments[ws.Segment]
			if ws.Offset+ws.Size > uint64(len(seg.Data)) {
				return nil, fmt.Errorf("loadwasm: %s: data symbol %s extends past segment", pn, ws.Name)
			}
			data := append([]byte(nil), seg.Data[ws.Offset:ws.Offset+ws.Size]...)
			kind := sym.SNOPTRDATA
			allZero := true
			for _, c := range data {
				allZero = allZero && c == 0
			}
			if allZero {
				kind = sym.SNOPTRBSS
				data = nil
			}
			bld.SetType(kind)
			bld.SetData(data)
			bld.SetSize(int64(ws.Size))
			align := int32(1)
			if seg.Align < 31 {
				align <<= seg.Align
			}
			bld.SetAlign(align)
		case wasmobj.SymbolGlobal:
			return nil, fmt.Errorf("loadwasm: %s: defined WebAssembly global %s is not supported", pn, ws.Name)
		}
	}

	for _, wr := range o.Relocations {
		if wr.Section != o.CodeSection {
			continue // DWARF and data relocations are handled in later chunks.
		}
		fn, off, ok := enclosingFunction(o.Functions, wr.Offset)
		if !ok {
			return nil, fmt.Errorf("loadwasm: %s: CODE relocation at %#x is outside a function", pn, wr.Offset)
		}
		var owner loader.Sym
		for i, ws := range o.Symbols {
			if ws.Kind == wasmobj.SymbolFunction && ws.Flags&wasmobj.SymUndefined == 0 && ws.Index == fn.Index {
				owner = syms[i]
				break
			}
		}
		if owner == 0 {
			return nil, fmt.Errorf("loadwasm: %s: relocation owner for function %d not found", pn, fn.Index)
		}
		bld := l.MakeSymbolUpdater(owner)
		if int(off) >= len(bld.Data()) {
			return nil, fmt.Errorf("loadwasm: %s: relocation offset outside %s", pn, bld.Name())
		}

		var typ objabi.RelocType
		var target loader.Sym
		var size uint8
		add := wr.Addend
		switch wr.Type {
		case wasmobj.RFunctionIndexLEB:
			typ = objabi.R_WASM_CALL
			target, err = symbolTarget(syms, wr.Index)
			size = lebSize(bld.Data()[off:])
		case wasmobj.RMemoryAddrLEB, wasmobj.RMemoryAddrSLEB, wasmobj.RMemoryAddrRelSLEB:
			typ = objabi.R_WASM_ADDR_LEB
			target, err = symbolTarget(syms, wr.Index)
			size = lebSize(bld.Data()[off:])
		case wasmobj.RMemoryAddrI32:
			typ = objabi.R_WASM_ADDR_I32
			target, err = symbolTarget(syms, wr.Index)
			size = 4
		case wasmobj.RGlobalIndexLEB:
			target, err = symbolTarget(syms, wr.Index)
			size = lebSize(bld.Data()[off:])
			name := l.SymName(target)
			if name == "__stack_pointer" {
				typ = objabi.R_WASM_GLOBAL_INDEX
				target = 0
			} else {
				if off == 0 || bld.Data()[off-1] != 0x23 { // global.get
					return nil, fmt.Errorf("loadwasm: %s: unsupported global relocation for %s", pn, name)
				}
				bld.Data()[off-1] = 0x41 // i32.const
				if name == "__memory_base" || name == "__table_base" {
					typ = objabi.R_WASM_CONST
					target = 0
				} else if symKinds[target] == wasmobj.SymbolFunction {
					typ = objabi.R_WASM_TABLE_INDEX
				} else {
					typ = objabi.R_WASM_ADDR_LEB
				}
			}
		case wasmobj.RTypeIndexLEB:
			if int(wr.Index) >= len(typeSyms) {
				err = fmt.Errorf("type index %d is out of range", wr.Index)
			} else {
				target = typeSyms[wr.Index]
			}
			typ = objabi.R_WASM_TYPE_INDEX
			size = lebSize(bld.Data()[off:])
		case wasmobj.RTableIndexSLEB, wasmobj.RTableIndexRelSLEB:
			typ = objabi.R_WASM_TABLE_INDEX
			target, err = symbolTarget(syms, wr.Index)
			size = lebSize(bld.Data()[off:])
		case wasmobj.RTableNumberLEB:
			typ = objabi.R_WASM_TABLE_NUMBER
			size = lebSize(bld.Data()[off:])
		default:
			return nil, fmt.Errorf("loadwasm: %s: unsupported CODE relocation type %d", pn, wr.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("loadwasm: %s: %v", pn, err)
		}
		r, _ := bld.AddRel(typ)
		r.SetOff(int32(off))
		r.SetSiz(size)
		r.SetSym(target)
		r.SetAdd(add)
	}
	for _, s := range textp {
		l.MakeSymbolUpdater(s).SortRelocs()
	}
	return textp, nil
}

func goFuncType(t wasmobj.FuncType) (obj.WasmFuncType, error) {
	convert := func(types []byte) ([]obj.WasmField, error) {
		fields := make([]obj.WasmField, len(types))
		for i, typ := range types {
			switch typ {
			case 0x7f:
				fields[i].Type = obj.WasmI32
			case 0x7e:
				fields[i].Type = obj.WasmI64
			case 0x7d:
				fields[i].Type = obj.WasmF32
			case 0x7c:
				fields[i].Type = obj.WasmF64
			case 0x7b:
				fields[i].Type = obj.WasmV128
			default:
				return nil, fmt.Errorf("unsupported value type %#x", typ)
			}
		}
		return fields, nil
	}
	params, err := convert(t.Params)
	if err != nil {
		return obj.WasmFuncType{}, err
	}
	results, err := convert(t.Results)
	return obj.WasmFuncType{Params: params, Results: results}, err
}

func enclosingFunction(functions []wasmobj.Function, off uint64) (wasmobj.Function, uint64, bool) {
	for _, fn := range functions {
		if off >= fn.CodeOffset && off < fn.CodeOffset+uint64(len(fn.Body)) {
			return fn, off - fn.CodeOffset, true
		}
	}
	return wasmobj.Function{}, 0, false
}

func symbolTarget(syms []loader.Sym, index uint32) (loader.Sym, error) {
	if int(index) >= len(syms) || syms[index] == 0 {
		return 0, fmt.Errorf("symbol index %d is out of range", index)
	}
	return syms[index], nil
}

func lebSize(b []byte) uint8 {
	for i, c := range b {
		if c&0x80 == 0 {
			return uint8(i + 1)
		}
	}
	return 0
}
