// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stdmethods_test

import (
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis/analysistest"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/stdmethods"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, stdmethods.Analyzer, "a", "typeparams")
}

func TestAnalyzeEncodingXML(t *testing.T) {
	analysistest.Run(t, "", stdmethods.Analyzer, "encoding/xml")
}
