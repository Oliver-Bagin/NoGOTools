// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unusedwrite_test

import (
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis/analysistest"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/unusedwrite"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, unusedwrite.Analyzer, "a", "importsunsafe")
}
