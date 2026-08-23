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
		name := ws.Name
		if ws.Flags&wasmobj.SymBindingLocal != 0 {
			version = localSymVersion
			// wasm-ld -r keeps file-local helpers from every selected archive
			// member, so one aggregate can legitimately contain several local
			// functions called (for example) __syscall4. A single Go object
			// version is not enough to distinguish them; give each symbol-table
			// entry a private loader name while retaining the original name in
			// the emitted wasm name section only as diagnostic metadata.
			name = fmt.Sprintf("%s.wasm.%d", ws.Name, i)
		}
		var s loader.Sym
		if ws.Name == "__heap_end" && ws.Flags&wasmobj.SymUndefined != 0 {
			// The target musl initializes its serialized sbrk cursor from
			// wasm-ld's synthetic __heap_end. Go owns final data layout, and
			// runtime.end is its equivalent boundary after both Go and native
			// static data.
			s = l.LookupOrCreateSym("runtime.end", 0)
		} else {
			s = l.LookupOrCreateCgoExport(name, version)
		}
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

	// wasm-ld marks modern TLS segments with WASM_SEG_FLAG_TLS. It also
	// accepts older objects where only TLS-relative relocations identify the
	// segment, so recover that information in the same way here. The -r
	// aggregate produced by the distro linker currently needs this fallback.
	tlsSegments := make(map[uint32]bool)
	for _, wr := range o.Relocations {
		if wr.Type != wasmobj.RMemoryAddrTLSSLEB && wr.Type != wasmobj.RMemoryAddrTLSSLEB64 {
			continue
		}
		if int(wr.Index) >= len(o.Symbols) {
			return nil, fmt.Errorf("loadwasm: %s: TLS relocation symbol %d is out of range", pn, wr.Index)
		}
		ws := o.Symbols[wr.Index]
		if ws.Kind != wasmobj.SymbolData || ws.Flags&wasmobj.SymUndefined != 0 {
			return nil, fmt.Errorf("loadwasm: %s: TLS relocation targets non-TLS symbol %s", pn, ws.Name)
		}
		tlsSegments[ws.Segment] = true
	}
	for i, seg := range o.Segments {
		if seg.Flags&wasmobj.SegmentFlagTLS != 0 {
			tlsSegments[uint32(i)] = true
		}
	}
	tlsBase := make(map[uint32]uint64)
	var tlsData []byte
	var tlsAlign uint32 = 1
	for i, seg := range o.Segments {
		if !tlsSegments[uint32(i)] {
			continue
		}
		align := uint32(1)
		if seg.Align < 31 {
			align <<= seg.Align
		}
		if align > tlsAlign {
			tlsAlign = align
		}
		for uint32(len(tlsData))&(align-1) != 0 {
			tlsData = append(tlsData, 0)
		}
		tlsBase[uint32(i)] = uint64(len(tlsData))
		tlsData = append(tlsData, seg.Data...)
	}
	if len(tlsSegments) != 0 {
		l.SetWasmTLS(tlsData, tlsAlign)
	}

	for _, imp := range o.Imports {
		// The target ABI's kernel functions are genuine module imports.
		// env imports in relocatable objects are linker-synthesized values or
		// Go/native bridge symbols and must remain available for resolution.
		if int(imp.TypeIndex) >= len(o.Types) {
			return nil, fmt.Errorf("loadwasm: %s: import %s.%s has invalid type %d", pn, imp.Module, imp.Name, imp.TypeIndex)
		}
		ft, err := goFuncType(o.Types[imp.TypeIndex])
		if err != nil {
			return nil, fmt.Errorf("loadwasm: %s: import %s.%s: %v", pn, imp.Module, imp.Name, err)
		}
		for i, ws := range o.Symbols {
			if ws.Kind != wasmobj.SymbolFunction || ws.Index != imp.Index || syms[i] == 0 {
				continue
			}
			if imp.Module == "env" {
				// An env import can be a reference from C to a function whose
				// body is supplied by Go. In particular, cmd/cgo declares each
				// _cgoexp_* callback as void(void*) only so C can take its table
				// address; the actual body has Go's (i32)->i32 continuation ABI.
				// An undefined declaration never owns the final definition's
				// type. A native definition in another aggregate supplies its
				// own type when loaded, and explicit Go-native adapters have fixed
				// types in cmd/link/internal/wasm.
				break
			}
			l.SetWasmTypeSym(syms[i], typeSyms[imp.TypeIndex])
			l.SetWasmHostImport(syms[i], obj.WasmImport{
				Module:       imp.Module,
				Name:         imp.Name,
				WasmFuncType: ft,
			})
			break
		}
	}

	for _, init := range o.InitFunctions {
		if int(init.Symbol) >= len(syms) || syms[init.Symbol] == 0 || o.Symbols[init.Symbol].Kind != wasmobj.SymbolFunction {
			return nil, fmt.Errorf("loadwasm: %s: invalid init function symbol %d", pn, init.Symbol)
		}
		l.AddWasmInitFunc(init.Priority, syms[init.Symbol])
	}

	functions := make(map[uint32]wasmobj.Function, len(o.Functions))
	for _, fn := range o.Functions {
		functions[fn.Index] = fn
	}
	var textp []loader.Sym
	definedFunctions := make(map[loader.Sym]bool)
	for i, ws := range o.Symbols {
		if ws.Flags&wasmobj.SymUndefined != 0 || syms[i] == 0 {
			continue
		}
		bld := l.MakeSymbolUpdater(syms[i])
		switch ws.Kind {
		case wasmobj.SymbolFunction:
			// A relocatable wasm symbol table may contain both an explicit
			// and an inferred entry for the same name/index. They resolve to
			// one Loader symbol; appending that symbol twice would assign it
			// two PCs and retroactively make Textp non-monotonic.
			if definedFunctions[syms[i]] {
				continue
			}
			definedFunctions[syms[i]] = true
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
			if tlsSegments[ws.Segment] {
				// TLS symbols denote offsets from the per-instance __tls_base,
				// not addresses in shared linear data. Their relocations are
				// converted to constants below.
				continue
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

	// Empty init/fini arrays are represented by equal weak boundary symbols.
	// The aggregate object references the fini pair even when no destructor is
	// selected, so materialize them as zero-sized data instead of leaving host
	// objects unresolved.
	for _, name := range []string{"__init_array_start", "__init_array_end", "__fini_array_start", "__fini_array_end"} {
		s := l.Lookup(name, 0)
		if s == 0 || (l.SymType(s) != sym.Sxxx && l.SymType(s) != sym.SHOSTOBJ) {
			continue
		}
		bld := l.MakeSymbolUpdater(s)
		bld.SetType(sym.SNOPTRDATA)
		bld.SetSize(0)
		bld.SetAlign(4)
	}

	ctors := synthNativeFunction(l, "__wasm_call_ctors")
	if ctors != 0 {
		for _, init := range l.WasmInitFuncs() {
			r, _ := l.MakeSymbolUpdater(ctors).AddRel(objabi.R_WASM_CALL)
			r.SetSym(init.Sym)
			// The final wasm encoder replaces this placeholder with the actual
			// ordered call sequence. A non-zero relocation makes deadcode retain
			// the constructor in the meantime.
			r.SetOff(0)
			r.SetSiz(1)
		}
		textp = append(textp, ctors)
	}
	if tls := synthNativeFunction(l, "__wasm_init_tls"); tls != 0 {
		textp = append(textp, tls)
	}

	for _, wr := range o.Relocations {
		if wr.Section == o.DataSection {
			segIndex, segOff, ok := enclosingDataSegment(o.Segments, wr.Offset)
			if !ok {
				return nil, fmt.Errorf("loadwasm: %s: DATA relocation at %#x is outside a segment", pn, wr.Offset)
			}
			ownerIndex, ownerOff, ok := enclosingDataSymbol(o.Symbols, segIndex, segOff, 4)
			if !ok || syms[ownerIndex] == 0 {
				return nil, fmt.Errorf("loadwasm: %s: DATA relocation at %#x has no symbol owner", pn, wr.Offset)
			}
			bld := l.MakeSymbolUpdater(syms[ownerIndex])
			// A zero-filled input symbol is normally represented as BSS with no
			// backing bytes. A relocation makes the final contents non-zero, so
			// materialize the symbol before attaching the relocation or Datblk
			// will reserve it without emitting the relocated initializer.
			if bld.Type() == sym.SNOPTRBSS {
				bld.SetType(sym.SNOPTRDATA)
				bld.SetData(make([]byte, bld.Size()))
			}
			target, err := symbolTarget(syms, wr.Index)
			if err != nil {
				return nil, fmt.Errorf("loadwasm: %s: %v", pn, err)
			}
			var typ objabi.RelocType
			switch wr.Type {
			case wasmobj.RMemoryAddrI32:
				typ = objabi.R_ADDR
			case wasmobj.RTableIndexI32:
				typ = objabi.R_WASM_TABLE_INDEX_I32
			default:
				return nil, fmt.Errorf("loadwasm: %s: unsupported DATA relocation type %d", pn, wr.Type)
			}
			r, _ := bld.AddRel(typ)
			r.SetOff(int32(ownerOff))
			r.SetSiz(4)
			r.SetSym(target)
			r.SetAdd(wr.Addend)
			continue
		}
		if wr.Section != o.CodeSection {
			continue // Relocated DWARF is not emitted for native objects yet.
		}
		fn, off, ok := enclosingFunction(o.Functions, wr.Offset)
		if !ok {
			return nil, fmt.Errorf("loadwasm: %s: CODE relocation at %#x is outside a function", pn, wr.Offset)
		}
		var owners []loader.Sym
		seenOwners := make(map[loader.Sym]bool)
		for i, ws := range o.Symbols {
			if ws.Kind == wasmobj.SymbolFunction && ws.Flags&wasmobj.SymUndefined == 0 && ws.Index == fn.Index && !seenOwners[syms[i]] {
				owners = append(owners, syms[i])
				seenOwners[syms[i]] = true
			}
		}
		if len(owners) == 0 {
			return nil, fmt.Errorf("loadwasm: %s: relocation owner for function %d not found", pn, fn.Index)
		}
		owner := owners[0]
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
			if isUndefinedWeak(o.Symbols, wr.Index) {
				// Static ELF linkers resolve an absent weak data symbol to
				// address zero. musl uses this for optional hooks such as
				// _DYNAMIC, so preserve the same contract in the final wasm
				// link instead of diagnosing an unresolved host object.
				typ = objabi.R_WASM_CONST
				target = 0
			} else {
				typ = objabi.R_WASM_ADDR_LEB
				target, err = symbolTarget(syms, wr.Index)
			}
			size = lebSize(bld.Data()[off:])
		case wasmobj.RMemoryAddrTLSSLEB:
			if int(wr.Index) >= len(o.Symbols) {
				err = fmt.Errorf("TLS symbol index %d is out of range", wr.Index)
				break
			}
			ws := o.Symbols[wr.Index]
			base, ok := tlsBase[ws.Segment]
			if !ok {
				err = fmt.Errorf("TLS relocation targets non-TLS symbol %s", ws.Name)
				break
			}
			typ = objabi.R_WASM_CONST
			target = 0
			add += int64(base + ws.Offset)
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
			} else if name == "__tls_base" {
				// Go's eight register globals occupy indices 0 through 7.
				// Keep LLVM TLS independent of Go's g global and let musl set
				// this ninth per-instance mutable global at thread entry.
				typ = objabi.R_WASM_GLOBAL_INDEX
				target = 0
				add += 8
			} else {
				if off == 0 || bld.Data()[off-1] != 0x23 { // global.get
					return nil, fmt.Errorf("loadwasm: %s: unsupported global relocation for %s", pn, name)
				}
				bld.Data()[off-1] = 0x41 // i32.const
				if name == "__memory_base" || name == "__table_base" || name == "__tls_size" || name == "__tls_align" {
					typ = objabi.R_WASM_CONST
					target = 0
					if name == "__tls_size" {
						add += int64(len(tlsData))
					} else if name == "__tls_align" && len(tlsSegments) != 0 {
						add += int64(tlsAlign)
					}
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
		// The wasm linking ABI permits several symbols (usually a strong
		// definition and one or more weak aliases) to name the same function
		// index. Dead-code elimination may retain any one of those aliases, so
		// every symbol carrying a copy of the body must carry the relocations as
		// well. Otherwise the retained alias still contains wasm-ld's padded
		// placeholder operand.
		for _, alias := range owners[1:] {
			aliasBld := l.MakeSymbolUpdater(alias)
			if off > 0 {
				aliasBld.Data()[off-1] = bld.Data()[off-1]
			}
			ar, _ := aliasBld.AddRel(typ)
			ar.SetOff(int32(off))
			ar.SetSiz(size)
			ar.SetSym(target)
			ar.SetAdd(add)
		}
	}
	for _, s := range textp {
		l.MakeSymbolUpdater(s).SortRelocs()
	}
	// DATA owners are not part of textp, but deterministic relocation order is
	// still required by the generic data emitter.
	for i, ws := range o.Symbols {
		if ws.Kind == wasmobj.SymbolData && syms[i] != 0 && l.SymType(syms[i]) != sym.Sxxx {
			l.MakeSymbolUpdater(syms[i]).SortRelocs()
		}
	}
	return textp, nil
}

func synthNativeFunction(l *loader.Loader, name string) loader.Sym {
	s := l.Lookup(name, 0)
	if s == 0 || (l.SymType(s) != sym.Sxxx && l.SymType(s) != sym.SHOSTOBJ) {
		return 0
	}
	bld := l.MakeSymbolUpdater(s)
	bld.SetType(sym.STEXT)
	bld.SetData([]byte{0, 0x0b}) // zero locals; end
	bld.SetSize(2)
	bld.SetExternal(true)
	bld.SetOnList(true)
	return s
}

func enclosingDataSegment(segments []wasmobj.DataSegment, off uint64) (uint32, uint64, bool) {
	for i, seg := range segments {
		if off >= seg.DataOffset && off < seg.DataOffset+uint64(len(seg.Data)) {
			return uint32(i), off - seg.DataOffset, true
		}
	}
	return 0, 0, false
}

func enclosingDataSymbol(symbols []wasmobj.Symbol, segment uint32, off, size uint64) (int, uint64, bool) {
	for i, s := range symbols {
		if s.Kind == wasmobj.SymbolData && s.Flags&wasmobj.SymUndefined == 0 && s.Segment == segment && off >= s.Offset && off+size <= s.Offset+s.Size {
			return i, off - s.Offset, true
		}
	}
	return 0, 0, false
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

func isUndefinedWeak(syms []wasmobj.Symbol, index uint32) bool {
	return int(index) < len(syms) &&
		syms[index].Flags&(wasmobj.SymUndefined|wasmobj.SymBindingWeak) ==
			wasmobj.SymUndefined|wasmobj.SymBindingWeak
}

func lebSize(b []byte) uint8 {
	for i, c := range b {
		if c&0x80 == 0 {
			return uint8(i + 1)
		}
	}
	return 0
}
