// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package analysisflags_test

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/tinygo-org/tinygo/x-tools/go/analysis"
	"github.com/tinygo-org/tinygo/x-tools/go/analysis/internal/analysisflags"
)

func main() {
	fmt.Println(analysisflags.Parse([]*analysis.Analyzer{
		{Name: "a1", Doc: "a1"},
		{Name: "a2", Doc: "a2"},
		{Name: "a3", Doc: "a3"},
	}, true))
	os.Exit(0)
}

// This test fork/execs the main function above.
func TestExec(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("skipping fork/exec test on this platform")
	}

	progname := os.Args[0]

	if os.Getenv("ANALYSISFLAGS_CHILD") == "1" {
		// child process
		os.Args = strings.Fields(progname + " " + os.Getenv("FLAGS"))
		main()
		panic("unreachable")
	}

	for _, test := range []struct {
		flags string
		want  string // output should contain want
	}{
		{"", "[a1 a2 a3]"},
		{"-a1=0", "[a2 a3]"},
		{"-a1=1", "[a1]"},
		{"-a1", "[a1]"},
		{"-a1=1 -a3=1", "[a1 a3]"},
		{"-a1=1 -a3=0", "[a1]"},
		{"-V=full", "analysisflags.test version devel"},
	} {
		cmd := exec.Command(progname, "-test.run=TestExec")
		cmd.Env = append(os.Environ(), "ANALYSISFLAGS_CHILD=1", "FLAGS="+test.flags)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("exec failed: %v; output=<<%s>>", err, output)
		}

		got := strings.TrimSpace(string(output))
		if !strings.Contains(got, test.want) {
			t.Errorf("got %q, does not contain %q", got, test.want)
		}
	}
}
