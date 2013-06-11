// Copyright 2013 Bobby Powers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamo

import (
	"bytes"
	"fmt"
	"go/token"
)

func Parse(f *token.File, fset *token.FileSet, str string) (*File, error) {
	parser := newParser(f, fset, newLex(str, f))
	result, nerr := parser.Parse()
	if nerr != 0 {
		return nil, fmt.Errorf("%d parse errors:\n%s", nerr,
			parser.errBuf.String())
	}

	return result, nil
}

type dynParser struct {
	tokf   *token.File
	fset   *token.FileSet
	lex    *dynLex
	f      *File
	errBuf bytes.Buffer
	nerr   int
}

func newParser(f *token.File, fs *token.FileSet, l *dynLex) *dynParser {
	return &dynParser{tokf: f, fset: fs, lex: l, f: new(File)}
}

func id(n string) *Ident {
	return &Ident{Name: n}
}

func (p *dynParser) Parse() (*File, int) {

	p.f.Name = id("main")
	p.declModel(p.f.Name)

	return p.f, p.nerr
}

func (p *dynParser) declModel(n *Ident) {
	m := new(ModelDecl)
	m.Name = n
	m.Body = new(BlockStmt)

	for {
		tok := p.lex.Peek()
		if tok.kind == itemIdentifier && len(tok.val) == 1 {
			p.stmtInto(m)
		} else {
			p.errBuf.WriteString(fmt.Sprintf("%s: expected 1 char ident, not '%s'\n",
				p.fset.Position(tok.pos), tok.val))
			p.nerr++
			break
		}
	}
}

func (p *dynParser) stmtInto(m *ModelDecl) {

	tok := p.lex.Token()
	if tok.kind != itemIdentifier {
		p.errBuf.WriteString("")
	}
}
