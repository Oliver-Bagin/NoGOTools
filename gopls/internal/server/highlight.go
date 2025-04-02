// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/file"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/golang"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/label"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/protocol"
	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/template"
	"github.com/tinygo-org/tinygo/x-tools/internal/event"
)

func (s *server) DocumentHighlight(ctx context.Context, params *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	ctx, done := event.Start(ctx, "lsp.Server.documentHighlight", label.URI.Of(params.TextDocument.URI))
	defer done()

	fh, snapshot, release, err := s.fileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()

	switch snapshot.FileKind(fh) {
	case file.Tmpl:
		return template.Highlight(ctx, snapshot, fh, params.Position)
	case file.Go:
		rngs, err := golang.Highlight(ctx, snapshot, fh, params.Position)
		if err != nil {
			event.Error(ctx, "no highlight", err)
		}
		return rngs, nil
	}
	return nil, nil // empty result
}
