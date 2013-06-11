// Copyright 2013 Bobby Powers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamo

import (
	"fmt"
	"go/token"
)

func Parse(f *token.File, fset *token.FileSet, str string) (*File, error) {
	parser := newParser(f, fset, newLex(str, f))
	result, nerr := parser.Parse()
	if nerr != 0 {
		return nil, fmt.Errorf("%d parse errors", nerr)
	}

	return result, nil
}

type dynParser struct {
	f    *token.File
	fset *token.FileSet
	lex  *dynLex
}

func newParser(f *token.File, fs *token.FileSet, l *dynLex) *dynParser {
	return &dynParser{f: f, fset: fs, lex: l}
}

func (p *dynParser) Parse() (*File, int) {
	for {
		t := p.lex.Token()
		if t.kind == itemEOF {
			break
		}
		fmt.Printf("%s: %s\n", p.fset.Position(t.pos), t)
	}
	return nil, 1
}
