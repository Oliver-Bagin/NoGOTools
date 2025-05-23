// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nilness_test

import (
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis/analysistest"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/nilness"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, nilness.Analyzer, "a")
}

func TestNilness(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, nilness.Analyzer, "b")
}

func TestInstantiated(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, nilness.Analyzer, "c")
}

func TestTypeSet(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, nilness.Analyzer, "d")
}
