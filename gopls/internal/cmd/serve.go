// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/cache"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/debug"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/lsprpc"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/protocol"
	"github.com/tinygo-org/tinygo/x-tools/internal/fakenet"
	"github.com/tinygo-org/tinygo/x-tools/internal/jsonrpc2"
	"github.com/tinygo-org/tinygo/x-tools/internal/tool"
)

// Serve is a struct that exposes the configurable parts of the LSP server as
// flags, in the right form for tool.Main to consume.
type Serve struct {
	Logfile     string        `flag:"logfile" help:"filename to log to. if value is \"auto\", then logging to a default output file is enabled"`
	Mode        string        `flag:"mode" help:"no effect"`
	Port        int           `flag:"port" help:"port on which to run gopls for debugging purposes"`
	Address     string        `flag:"listen" help:"address on which to listen for remote connections. If prefixed by 'unix;', the subsequent address is assumed to be a unix domain socket. Otherwise, TCP is used."`
	IdleTimeout time.Duration `flag:"listen.timeout" help:"when used with -listen, shut down the server when there are no connected clients for this duration"`
	Trace       bool          `flag:"rpc.trace" help:"print the full rpc trace in lsp inspector format"`
	Debug       string        `flag:"debug" help:"serve debug information on the supplied address"`

	RemoteListenTimeout time.Duration `flag:"remote.listen.timeout" help:"when used with -remote=auto, the -listen.timeout value used to start the daemon"`
	RemoteDebug         string        `flag:"remote.debug" help:"when used with -remote=auto, the -debug value used to start the daemon"`
	RemoteLogfile       string        `flag:"remote.logfile" help:"when used with -remote=auto, the -logfile value used to start the daemon"`

	app *Application
}

func (s *Serve) Name() string   { return "serve" }
func (s *Serve) Parent() string { return s.app.Name() }
func (s *Serve) Usage() string  { return "[server-flags]" }
func (s *Serve) ShortHelp() string {
	return "run a server for Go code using the Language Server Protocol"
}
func (s *Serve) DetailedHelp(f *flag.FlagSet) {
	fmt.Fprint(f.Output(), `  gopls [flags] [server-flags]

The server communicates using JSONRPC2 on stdin and stdout, and is intended to be run directly as
a child of an editor process.

server-flags:
`)
	printFlagDefaults(f)
}

func (s *Serve) remoteArgs(network, address string) []string {
	args := []string{"serve",
		"-listen", fmt.Sprintf(`%s;%s`, network, address),
	}
	if s.RemoteDebug != "" {
		args = append(args, "-debug", s.RemoteDebug)
	}
	if s.RemoteListenTimeout != 0 {
		args = append(args, "-listen.timeout", s.RemoteListenTimeout.String())
	}
	if s.RemoteLogfile != "" {
		args = append(args, "-logfile", s.RemoteLogfile)
	}
	return args
}

// Run configures a server based on the flags, and then runs it.
// It blocks until the server shuts down.
func (s *Serve) Run(ctx context.Context, args ...string) error {
	if len(args) > 0 {
		return tool.CommandLineErrorf("server does not take arguments, got %v", args)
	}

	di := debug.GetInstance(ctx)
	isDaemon := s.Address != "" || s.Port != 0
	if di != nil {
		closeLog, err := di.SetLogFile(s.Logfile, isDaemon)
		if err != nil {
			return err
		}
		defer closeLog()
		di.ServerAddress = s.Address
		di.Serve(ctx, s.Debug)
	}
	var ss jsonrpc2.StreamServer
	if s.app.Remote != "" {
		var err error
		ss, err = lsprpc.NewForwarder(s.app.Remote, s.remoteArgs)
		if err != nil {
			return fmt.Errorf("creating forwarder: %w", err)
		}
	} else {
		ss = lsprpc.NewStreamServer(cache.New(nil), isDaemon, s.app.options)
	}

	var network, addr string
	if s.Address != "" {
		network, addr = lsprpc.ParseAddr(s.Address)
	}
	if s.Port != 0 {
		network = "tcp"
		// TODO(adonovan): should gopls ever be listening on network
		// sockets, or only local ones?
		//
		// Ian says this was added in anticipation of
		// something related to "VS Code remote" that turned
		// out to be unnecessary. So I propose we limit it to
		// localhost, if only so that we avoid the macOS
		// firewall prompt.
		//
		// Hana says: "s.Address is for the remote access (LSP)
		// and s.Port is for debugging purpose (according to
		// the Server type documentation). I am not sure why the
		// existing code here is mixing up and overwriting addr.
		// For debugging endpoint, I think localhost makes perfect sense."
		//
		// TODO(adonovan): disentangle Address and Port,
		// and use only localhost for the latter.
		addr = fmt.Sprintf(":%v", s.Port)
	}
	if addr != "" {
		log.Printf("Gopls daemon: listening on %s network, address %s...", network, addr)
		defer log.Printf("Gopls daemon: exiting")
		return jsonrpc2.ListenAndServe(ctx, network, addr, ss, s.IdleTimeout)
	}
	stream := jsonrpc2.NewHeaderStream(fakenet.NewConn("stdio", os.Stdin, os.Stdout))
	if s.Trace && di != nil {
		stream = protocol.LoggingStream(stream, di.LogWriter)
	}
	conn := jsonrpc2.NewConn(stream)
	err := ss.ServeStream(ctx, conn)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
