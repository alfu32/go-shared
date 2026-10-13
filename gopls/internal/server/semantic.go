// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	goxlsp "github.com/alfu32/go-shared/gox/lsp"
	"golang.org/x/tools/gopls/internal/file"
	"golang.org/x/tools/gopls/internal/golang"
	"golang.org/x/tools/gopls/internal/label"
	"golang.org/x/tools/gopls/internal/protocol"
	"golang.org/x/tools/gopls/internal/template"
	"golang.org/x/tools/internal/event"
)

func (s *server) SemanticTokensFull(ctx context.Context, params *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	return s.semanticTokens(ctx, params.TextDocument, nil)
}

func (s *server) SemanticTokensRange(ctx context.Context, params *protocol.SemanticTokensRangeParams) (*protocol.SemanticTokens, error) {
	return s.semanticTokens(ctx, params.TextDocument, &params.Range)
}

func (s *server) semanticTokens(ctx context.Context, td protocol.TextDocumentIdentifier, rng *protocol.Range) (*protocol.SemanticTokens, error) {
	ctx, done := event.Start(ctx, "server.semanticTokens", label.URI.Of(td.URI))
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, td.URI)
	if err != nil {
		return nil, err
	}
	defer release()

	switch snapshot.FileKind(fh) {
	case file.Tmpl:
		if snapshot.Options().SemanticTokens {
			return template.SemanticTokens(ctx, snapshot, fh.URI())
		}
	case file.Go:
		semanticTokens := snapshot.Options().SemanticTokens
		if !semanticTokens {
			if content, err := fh.Content(); err == nil {
				// Go's lexical highlighter sees the contents of F/X literals as a
				// string. Send semantic tokens for GoX files even when the optional
				// whole-Go semantic highlighting setting is disabled, so expressions
				// inside interpolation braces receive normal Go token colors.
				semanticTokens = len(goxlsp.GoXLiteralRanges(content)) > 0
			}
		}
		if semanticTokens {
			return golang.SemanticTokens(ctx, snapshot, fh, rng)
		}
	}

	// Not enabled, or unsupported file type: return empty result.
	//
	// Returning an empty response is necessary to invalidate
	// semantic tokens in VS Code (and perhaps other editors).
	// Previously, we returned an error, but that had the side effect
	// of noisy "semantictokens are disabled" logs on every keystroke.
	//
	// We must return a non-nil Data slice for JSON serialization.
	// We do not return an empty field with "omitempty" set,
	// as it is not marked optional in the protocol (golang/go#67885).
	return &protocol.SemanticTokens{Data: []uint32{}}, nil
}
