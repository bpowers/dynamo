// Copyright 2013 Bobby Powers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamo

import (
	"bytes"
	"fmt"
	"go/token"
	"strings"
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

func ident(tok Token) *Ident {
	return &Ident{tok.pos, tok.val, nil}
}

func id(n string) *Ident {
	return &Ident{Name: n}
}

func (p *dynParser) Parse() (*File, int) {

	p.f.Name = id("main")
	p.declModel(p.f.Name)

	return p.f, p.nerr
}

func (p *dynParser) errorf(tok Token, f string, args ...interface{}) {
	p.errBuf.WriteString(fmt.Sprintf("%s: %s\n", p.fset.Position(tok.pos),
		fmt.Sprintf(f, args...)))
	p.nerr++
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
			p.errorf(tok, "expected 1 char ident, not '%s'", tok.val)
			break
		}
	}
}

func (p *dynParser) stmtInto(m *ModelDecl) {
	typeTok := p.lex.Token()
	typeTok.val = strings.ToUpper(typeTok.val)
	switch typeTok.val {
	case "L", "N", "C", "R", "A", "T":
		decl, ok := p.varDecl(typeTok)
		if !ok || !p.consumeEqual() {
			p.discardStmt()
			return
		}
		expr, ok := p.expr()
		if !ok {
			p.discardStmt()
			return
		}
		assign := &AssignStmt{Lhs: decl, Rhs: expr}
		m.Body.List = append(m.Body.List, assign)
	default:
		p.errorf(typeTok, "unknown type: %s", typeTok.val)
	}
}

func (p *dynParser) expr() (Expr, bool) {
	fmt.Printf("expr\n")
	return nil, false
}

// discard everything before the next EOF or semi
func (p *dynParser) discardStmt() {
	tok := p.lex.Token()
	for tok.kind != itemEOF && tok.kind != itemSemi {
		fmt.Printf("discard: %s\n", tok)
		tok = p.lex.Token()
	}
}

func typeIdent(typeTok Token) *Ident {
	var n string
	switch typeTok.val {
	case "L":
		n = "stock"
	case "N":
		n = "initial"
	case "C":
		n = "constant"
	case "R":
		n = "flow"
	case "A":
		n = "aux"
	case "T":
		n = "table"
	default:
		panic("unknown type " + typeTok.val)
	}
	return &Ident{typeTok.pos, n, nil}
}

func (p *dynParser) varDecl(typeTok Token) (*VarDecl, bool) {
	nameTok := p.lex.Token()
	if nameTok.kind != itemIdentifier {
		p.errorf(nameTok, "expected ident, not %s", typeTok.val)
		return nil, false
	}
	d := new(VarDecl)
	d.Name = ident(nameTok)
	d.Type = typeIdent(typeTok)
	return d, true
}

func (p *dynParser) tableInto(m *ModelDecl) {

}

// consumeEqual returns true on success
func (p *dynParser) consumeEqual() bool {
	tok := p.lex.Token()
	if tok.val != "=" {
		p.errorf(tok, "expected =, not %s", tok.val)
		return false
	}
	return true
}
