// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package copylock_test

import (
	"path/filepath"
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis/analysistest"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/copylock"
	"github.com/tinygo-org/tinygo/x-tools/internal/testenv"
	"github.com/tinygo-org/tinygo/x-tools/internal/testfiles"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, copylock.Analyzer, "a", "typeparams", "issue67787", "unfortunate")
}

func TestVersions22(t *testing.T) {
	testenv.NeedsGo1Point(t, 22)

	dir := testfiles.ExtractTxtarFileToTmp(t, filepath.Join(analysistest.TestData(), "src", "forstmt", "go22.txtar"))
	analysistest.Run(t, dir, copylock.Analyzer, "golang.org/fake/forstmt")
}

func TestVersions21(t *testing.T) {
	dir := testfiles.ExtractTxtarFileToTmp(t, filepath.Join(analysistest.TestData(), "src", "forstmt", "go21.txtar"))
	analysistest.Run(t, dir, copylock.Analyzer, "golang.org/fake/forstmt")
}
