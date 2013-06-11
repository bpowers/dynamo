// Copyright 2013 Bobby Powers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynamo

import (
	"bytes"
	"fmt"
	"go/token"
	"log"
	"strings"
	"unicode"
	"unicode/utf8"
)

const eof = 0

type itemType int

const (
	itemEOF        itemType = iota
	itemIdentifier itemType = iota
	itemNumber     itemType = iota
	itemSemi       itemType = iota
	itemOperator   itemType = iota
	itemKindDecl   itemType = iota
	itemKeyword    itemType = iota
	itemLiteral    itemType = iota
	itemLBracket   itemType = iota
	itemRBracket   itemType = iota
	itemLParen     itemType = iota
	itemRParen     itemType = iota
	itemLSquare    itemType = iota
	itemRSquare    itemType = iota
)

func (i itemType) String() string {
	switch i {
	case itemEOF:
		return "eof"
	case itemIdentifier:
		return "ident"
	case itemNumber:
		return "num"
	case itemSemi:
		return "semi"
	case itemOperator:
		return "op"
	case itemKindDecl:
		return "kind"
	case itemKeyword:
		return "keyword"
	case itemLiteral:
		return "lit"
	case itemLBracket:
		return "lbrac"
	case itemRBracket:
		return "rbrac"
	case itemLParen:
		return "lparen"
	case itemRParen:
		return "rparen"
	case itemLSquare:
		return "lsquare"
	case itemRSquare:
		return "rsquare"
	default:
		return "unknown"
	}
}

type astType int

const (
	astRef astType = iota
	astNumber
	astOp
	astFn
)

// A Token provides information about a particular run of consecutive
// chars in a file.
type Token struct {
	pos  token.Pos
	val  string
	kind itemType
}

func (t Token) String() string {
	val := t.val
	if t.kind == itemSemi {
		val = ";"
	}
	return fmt.Sprintf("(%s %s)", t.kind, val)
}

type stateFn func() stateFn

type dynLex struct {
	f     *token.File
	s     string // the string being scanned
	pos   int    // current position in the input
	start int    // start of this token
	width int    // width of the last rune
	last  Token
	items chan Token // channel of scanned items
	state stateFn
	semi  bool
}

func (l *dynLex) Token() Token {
	for {
		select {
		case item := <-l.items:
			return item
		default:
			l.state = l.state()
		}
	}
	panic("unreachable")
}

func newLex(input string, file *token.File) *dynLex {
	l := new(dynLex)
	l.f = file
	l.s = input
	l.items = make(chan Token, 2)
	l.state = l.begin
	return l
}

func (l *dynLex) getLine(pos token.Position) string {
	p := pos.Offset - pos.Column
	if p < 0 || p >= len(l.s) {
		return fmt.Sprintf("getLine: o%d c%d, len%d",
			pos.Offset, pos.Column, len(l.s))
	}
	result := l.s[pos.Offset-pos.Column:]
	if newline := strings.IndexRune(result, '\n'); newline != -1 {
		result = result[:newline]
	}
	return result
}

func (l *dynLex) Error(s string) {
	pos := l.f.Position(l.last.pos)
	line := l.getLine(pos)
	// we want the number of spaces (taking into account tabs)
	// before the problematic token
	prefixLen := pos.Column + strings.Count(line[:pos.Column], "\t")*7 - 1
	prefix := strings.Repeat(" ", prefixLen)

	line = strings.Replace(line, "\t", "        ", -1)

	fmt.Printf("%s:%d:%d: error: %s\n", pos.Filename,
		pos.Line, pos.Column, s)
	fmt.Printf("%s\n", line)
	fmt.Printf("%s^\n", prefix)
}

func (l *dynLex) next() rune {
	if l.pos >= len(l.s) {
		return 0
	}
	r, width := utf8.DecodeRuneInString(l.s[l.pos:])
	l.pos += width
	l.width = width

	if r == '\n' {
		l.f.AddLine(l.pos + 1)
	}
	return r
}

func (l *dynLex) backup() {
	l.pos -= l.width
}

func (l *dynLex) peek() rune {
	peek := l.next()
	l.backup()
	return peek
}

func (l *dynLex) ignore() {
	l.start = l.pos
}

func (l *dynLex) accept(valid string) bool {
	if strings.IndexRune(valid, l.next()) >= 0 {
		return true
	}
	l.backup()
	return false
}

func (l *dynLex) acceptRun(valid string) {
	for strings.IndexRune(valid, l.next()) >= 0 {
	}
	l.backup()
}

func (l *dynLex) emit(ty itemType) {
	t := Token{
		pos:  l.f.Pos(l.pos),
		val:  l.s[l.start:l.pos],
		kind: ty,
	}
	//log.Printf("t: %#v\n", t)
	l.last = t
	l.items <- t
	l.ignore()

	switch {
	case ty == itemRBracket || ty == itemRParen || ty == itemRSquare:
		fallthrough
	case ty == itemIdentifier || ty == itemNumber || ty == itemKindDecl || ty == itemLiteral:
		l.semi = true
	default:
		l.semi = false
	}
}

func (l *dynLex) errorf(format string, args ...interface{}) stateFn {
	log.Printf(format, args...)
	l.emit(itemEOF)
	return nil
}

func (l *dynLex) begin() stateFn {
	switch r := l.next(); {
	case r == '*':
		return l.comment
	default:
		return l.errorf("Dynamo programs must begin with a *, not %#U\n", r)
	}
}

func (l *dynLex) statement() stateFn {
	switch r := l.next(); {
	case r == eof:
		if l.semi {
			l.emit(itemSemi)
		}
		l.emit(itemEOF)
	case r == '/':
		if l.peek() == '/' {
			l.next()
			return l.comment
		}
		if l.peek() == '*' {
			l.next()
			return l.multiComment
		}
		l.emit(itemOperator)
	case r == '`':
		return l.lexType
	case r == ';':
		l.emit(itemSemi)
	case unicode.IsSpace(r):
		if r == '\n' && l.semi {
			l.emit(itemSemi)
		}
		//		log.Print("1 ignoring:", l.s[l.start:l.pos])
		l.ignore()
	case unicode.IsDigit(r) || r == '.':
		l.backup()
		return l.number
	case isLiteralStart(r):
		l.backup()
		return l.literal
	case isIdentifierStart(r):
		l.backup()
		return l.identifier
	case isOperator(r):
		l.backup()
		return l.operator
	default:
		return l.errorf("unrecognized char: %#U\n", r)
	}
	return l.statement
}

func (l *dynLex) operator() stateFn {
	ty := itemOperator
	r := l.next()
	switch {
	case r == '{':
		ty = itemLBracket
	case r == '}':
		ty = itemRBracket
	case r == '(':
		ty = itemLParen
	case r == ')':
		ty = itemRParen
	case r == '[':
		ty = itemLSquare
	case r == ']':
		ty = itemRSquare
	}
	l.emit(ty)
	return l.statement
}

func (l *dynLex) comment() stateFn {
	// skip everything until the end of the line, or the end of
	// the file, whichever is first
	for r := l.next(); r != '\n' && r != eof; r = l.next() {
	}
	l.backup()
	//	log.Print("2 ignoring:", l.s[l.start:l.pos])
	l.ignore()
	return l.statement
}

func (l *dynLex) multiComment() stateFn {
	// skip everything until the end of the line, or the end of
	// the file, whichever is first
	for r := l.next(); ; r = l.next() {
		if r == eof {
			l.backup()
			break
		}
		if r != '*' {
			continue
		}
		if l.peek() == '/' {
			l.next()
			break
		}
	}
	//	log.Print("2 ignoring:", l.s[l.start:l.pos])
	l.ignore()
	return l.statement
}

func (l *dynLex) lexType() stateFn {
	l.ignore()
	for r := l.next(); r != '`' && r != eof; r = l.next() {
	}
	l.backup()

	if l.peek() != '`' {
		return l.errorf("unexpected EOF")
	}
	l.emit(itemKindDecl)
	l.next()
	l.ignore()
	return l.statement
}

func (l *dynLex) number() stateFn {
	l.acceptRun("0123456789")
	l.accept(".")
	l.acceptRun("0123456789")
	if l.accept("eE") {
		l.accept("+-")
		l.acceptRun("0123456789")
	}
	l.emit(itemNumber)
	return l.statement
}

func (l *dynLex) literal() stateFn {
	delim := l.next()
	l.ignore()
	for r := l.next(); r != delim && r != eof; r = l.next() {
	}
	l.backup()

	if l.peek() != delim {
		return l.errorf("unexpected EOF")
	}
	l.emit(itemLiteral)
	l.next()
	l.ignore()
	return l.statement
}

func (l *dynLex) identifier() stateFn {
	for isAlphaNumeric(l.next()) {
	}
	l.backup()
	switch id := l.s[l.start:l.pos]; {
	case id == "kind":
		l.emit(itemKeyword)
	case id == "import":
		l.emit(itemKeyword)
	case id == "package":
		l.emit(itemKeyword)
	case id == "model":
		l.emit(itemKeyword)
	case id == "interface":
		l.emit(itemKeyword)
	case id == "specializes":
		l.emit(itemKeyword)
	default:
		l.emit(itemIdentifier)
	}
	return l.statement
}

func isLiteralStart(r rune) bool {
	return r == '"'
}

func isOperator(r rune) bool {
	return bytes.IndexRune([]byte(",+-*/|&=(){}[]:"), r) > -1
}

func isIdentifierStart(r rune) bool {
	return !(unicode.IsDigit(r) || unicode.IsSpace(r) || isOperator(r))
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return !(unicode.IsSpace(r) || isOperator(r) || r == ';')
}
