// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unusedresult_test

import (
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis/analysistest"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/passes/unusedresult"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	funcs := "typeparams/userdefs.MustUse,errors.New,fmt.Errorf,fmt.Sprintf,fmt.Sprint"
	unusedresult.Analyzer.Flags.Set("funcs", funcs)
	analysistest.Run(t, testdata, unusedresult.Analyzer, "a", "typeparams")
}
