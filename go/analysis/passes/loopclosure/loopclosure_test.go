// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package loopclosure_test

import (
	"path/filepath"
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis/analysistest"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/loopclosure"
	"github.com/tinygo-org/tinygo/x-tools/internal/testfiles"
)

func TestVersions(t *testing.T) {
	dir := testfiles.ExtractTxtarFileToTmp(t, filepath.Join(analysistest.TestData(), "src", "versions", "go22.txtar"))
	analysistest.Run(t, dir, loopclosure.Analyzer, "golang.org/fake/versions")
}
