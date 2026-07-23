// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Printf("grandchild:%s\n", os.Getenv("GO_WASM_CHILD"))
}
