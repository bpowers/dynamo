// Copyright 2013 Bobby Powers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamo

import (
	"bytes"
	"fmt"
	"github.com/bpowers/boosd/runtime"
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
outer:
	for {
		switch tok := p.lex.Peek(); tok.kind {
		case itemEOF:
			break outer
		case itemSemi:
			p.lex.Token() // discard
		case itemIdentifier:
			if len(tok.val) == 1 {
				p.stmtInto(m)
				break
			}
			fallthrough
		default:
			p.errorf(tok, "expected 1 char ident, not '%s'", tok.val)
			p.lex.Token() // discard
		}
	}

	if n.Name == "main" {
		if err := extractTimespec(m); err != nil {
			p.errorf(Token{}, "extractTimespec: %s", err)
		}
	}

	p.f.Decls = append(p.f.Decls, m)
}

func floatLitS(t Token) *BasicLit {
	return &BasicLit{t.pos, token.FLOAT, t.val}
}

func floatLit(f float64) *BasicLit {
	// FIXME: be more precise or something
	return &BasicLit{Kind: token.FLOAT, Value: fmt.Sprintf("%f", f)}
}

func extractTimespec(m *ModelDecl) error {
	ts := new(AssignStmt)
	ts.Lhs = new(VarDecl)
	ts.Lhs.Name = id("timespec")
	rhs := new(CompositeLit)
	ts.Rhs = rhs

	spec := runtime.Timespec{
		DT:       1,
		SaveStep: 1,
	}

	for _, stmt := range m.Body.List {
		assign, ok := stmt.(*AssignStmt)
		if !ok {
			continue
		}
		var err error
		switch strings.ToUpper(assign.Lhs.Name.Name) {
		case "TIME":
			spec.Start, err = constEval(assign.Rhs)
		case "LENGTH":
			spec.End, err = constEval(assign.Rhs)
		case "SAVPER":
			spec.SaveStep, err = constEval(assign.Rhs)
		case "DT":
			spec.DT, err = constEval(assign.Rhs)
		}
		if err != nil {
			return fmt.Errorf("constEval(%s): %s", assign.Lhs.Name.Name, err)
		}
	}

	// remove these const assignments from the simulation, they
	// are purely to specify the timespec
	for i := 0; i < len(m.Body.List); i++ {
		assign, ok := m.Body.List[i].(*AssignStmt)
		if !ok {
			continue
		}
		switch strings.ToUpper(assign.Lhs.Name.Name) {
		case "TIME", "LENGTH", "SAVPER", "DT":
			m.Body.List = append(m.Body.List[:i], m.Body.List[i+1:]...)
			i--
		}
	}

	rhs.Elts = append(rhs.Elts, &KeyValueExpr{Key: id("start"), Value: floatLit(spec.Start)})
	rhs.Elts = append(rhs.Elts, &KeyValueExpr{Key: id("end"), Value: floatLit(spec.End)})
	rhs.Elts = append(rhs.Elts, &KeyValueExpr{Key: id("dt"), Value: floatLit(spec.DT)})
	rhs.Elts = append(rhs.Elts, &KeyValueExpr{Key: id("save_step"), Value: floatLit(spec.SaveStep)})

	m.Body.List = append(m.Body.List, ts)

	return nil
}

func (p *dynParser) stmtInto(m *ModelDecl) {
	typeTok := p.lex.Token()
	typeTok.val = strings.ToUpper(typeTok.val)
	switch typeTok.val {
	case "L", "N", "C", "R", "A":
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
		m.Body.List = append(m.Body.List, &AssignStmt{Lhs: decl, Rhs: expr})
	case "T":
		decl, ok := p.varDecl(typeTok)
		if !ok || !p.consumeEqual() {
			p.discardStmt()
			return
		}
		expr, ok := p.tableDef()
		if !ok {
			p.discardStmt()
			return
		}
		m.Body.List = append(m.Body.List, &AssignStmt{Lhs: decl, Rhs: expr})
	default:
		p.errorf(typeTok, "unknown type: %s", typeTok.val)
	}
}

func (p *dynParser) expr() (Expr, bool) {
	switch tok := p.lex.Token(); tok.kind {
	case itemNumber:
		return &BasicLit{tok.pos, token.FLOAT, tok.val}, true
	default:
		fmt.Printf("expr\n")
		return nil, false
	}
}

func (p *dynParser) term() (Expr, bool) {
	return nil, false
}

func (p *dynParser) factor() (Expr, bool) {
	return nil, false
}

func (p *dynParser) ident() (Expr, bool) {
	return nil, false
}

func (p *dynParser) num() (Expr, bool) {
	switch tok := p.lex.Token(); tok.kind {
	case itemNumber:
		return &BasicLit{tok.pos, token.FLOAT, tok.val}, true
	default:
		fmt.Printf("expr\n")
		return nil, false
	}
}

func (p *dynParser) tableDef() (Expr, bool) {
	table := new(TableFwdExpr)
outer:
	for {
		tok := p.lex.Token()
		if tok.kind != itemNumber {
			p.errorf(tok, "expected float literal in table def, not '%s'", tok.val)
			return nil, true
		}
		table.Ys = append(table.Ys, floatLitS(tok))

		switch tok = p.lex.Token(); {
		case tok.val == "/":
			break // discard
		case tok.kind == itemSemi || tok.kind == itemEOF:
			break outer
		default:
			p.errorf(tok, "expected '/' in table def, not '%s'", tok.val)
		}
	}
	return table, false
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
		n = "const"
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
