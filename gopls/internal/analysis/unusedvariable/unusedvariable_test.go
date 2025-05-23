// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unusedvariable_test

import (
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis/analysistest"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/analysis/unusedvariable"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()

	t.Run("decl", func(t *testing.T) {
		analysistest.RunWithSuggestedFixes(t, testdata, unusedvariable.Analyzer, "decl")
	})

	t.Run("assign", func(t *testing.T) {
		analysistest.RunWithSuggestedFixes(t, testdata, unusedvariable.Analyzer, "assign")
	})
}
