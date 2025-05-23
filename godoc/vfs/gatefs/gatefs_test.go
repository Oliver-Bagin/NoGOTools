// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gatefs_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/godoc/vfs"
	"github.com/tinygo-org/tinygo/x-tools/godoc/vfs/gatefs"
)

func TestRootType(t *testing.T) {
	goPath := os.Getenv("GOPATH")
	var expectedType vfs.RootType
	if goPath == "" {
		expectedType = ""
	} else {
		expectedType = vfs.RootTypeGoPath
	}
	tests := []struct {
		path   string
		fsType vfs.RootType
	}{
		{runtime.GOROOT(), vfs.RootTypeGoRoot},
		{goPath, expectedType},
		{"/tmp/", ""},
	}

	for _, item := range tests {
		fs := gatefs.New(vfs.OS(item.path), make(chan bool, 1))
		if fs.RootType("path") != item.fsType {
			t.Errorf("unexpected fsType. Expected- %v, Got- %v", item.fsType, fs.RootType("path"))
		}
	}
}
