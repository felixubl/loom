package loom

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxNesting caps how deeply source may nest, so a pathological input cannot
// overflow the parser's own stack. Evaluation has its own depth limit (§35).
const maxNesting = 512

// Parse reads the surface syntax of §4:
//
//	term := atom | variable | variable "=>" term | term "(" term ")"
//
// Application associates left, so f(x)(y) means (f(x))(y). There is no
// multi-argument call.
//
// An identifier is a variable when an enclosing lambda binds it and a reference
// otherwise. A reference nothing defines evaluates to the atom of its own name,
// which is why knows(Alice) is graph structure while x => x is a lambda over a
// variable.
//
// A newline terminates a statement that is complete there. Where the grammar
// still requires a term, parsing continues on the next line. An application's
// "(" only continues a term on the same line, so a line beginning with "("
// starts a new term rather than becoming an argument to the line above.
//
// Parse is store-free: it produces syntax, not values.
func Parse(src string) (Term, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	p.skipLines()
	t, err := p.term(0)
	if err != nil {
		return nil, err
	}
	p.skipLines()
	if p.peek().kind != tokEOF {
		return nil, &SyntaxError{Pos: p.peek().pos, Msg: "unexpected " + p.peek().describe()}
	}
	return t, nil
}

// ParsePattern reads the informal pattern syntax of §22:
//
//	knows(Alice)(?x)   ?relation(Alice)(Bob)   pair(?x)(?x)   _
//
// ?x is a pattern capture, not a lambda variable (§22). Unlike Parse this needs
// the store, because a pattern's constants name values that must live in it.
func (s *Store) ParsePattern(src string) (Pattern, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, store: s}
	p.skipLines()
	pat, err := p.pattern(0)
	if err != nil {
		return nil, err
	}
	p.skipLines()
	if p.peek().kind != tokEOF {
		return nil, &SyntaxError{Pos: p.peek().pos, Msg: "unexpected " + p.peek().describe()}
	}
	return pat, nil
}

type tokenKind uint8

const (
	tokEOF tokenKind = iota
	tokIdent
	tokInt
	tokText
	tokOpen
	tokClose
	tokArrow
	tokQuery
	tokAssign
	tokBraceOpen
	tokBraceClose
	tokColon
	tokNewline
)

type token struct {
	kind tokenKind
	text string
	num  int64
	pos  int
	line int
}

func (t token) describe() string {
	switch t.kind {
	case tokEOF:
		return "end of input"
	case tokOpen:
		return `"("`
	case tokClose:
		return `")"`
	case tokArrow:
		return `"=>"`
	case tokQuery:
		return `"?"`
	case tokAssign:
		return `"="`
	case tokBraceOpen:
		return `"{"`
	case tokBraceClose:
		return `"}"`
	case tokColon:
		return `":"`
	case tokNewline:
		return "end of line"
	}
	return strconv.Quote(t.text)
}

func lex(src string) ([]token, error) {
	var toks []token
	i, line, parens := 0, 1, 0
	emit := func(t token) { t.line = line; toks = append(toks, t) }
	// A newline ends a statement, except inside parentheses: explicit
	// delimiters are how a term deliberately spans lines. Runs of blank lines
	// collapse into one terminator.
	endLine := func(at int) {
		if parens == 0 && len(toks) > 0 && toks[len(toks)-1].kind != tokNewline {
			emit(token{kind: tokNewline, pos: at})
		}
	}
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			endLine(i)
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '(':
			emit(token{kind: tokOpen, pos: i})
			parens++
			i++
		case c == ')':
			emit(token{kind: tokClose, pos: i})
			if parens > 0 {
				parens--
			}
			i++
		case c == '?':
			emit(token{kind: tokQuery, pos: i})
			i++
		case c == '=' && i+1 < len(src) && src[i+1] == '>':
			emit(token{kind: tokArrow, pos: i})
			i += 2
		case c == '=':
			emit(token{kind: tokAssign, pos: i})
			i++
		case c == '{':
			emit(token{kind: tokBraceOpen, pos: i})
			i++
		case c == '}':
			emit(token{kind: tokBraceClose, pos: i})
			i++
		case c == ':':
			emit(token{kind: tokColon, pos: i})
			i++
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '"':
			text, next, err := unquoteText(src, i)
			if err != nil {
				return nil, err
			}
			emit(token{kind: tokText, text: text, pos: i})
			i = next
		case c == '-' || (c >= '0' && c <= '9'):
			tok, next, err := lexInt(src, i)
			if err != nil {
				return nil, err
			}
			emit(tok)
			i = next
		case isIdentStart(src, i):
			start := i
			for i < len(src) && isIdentPart(src, i) {
				_, w := utf8.DecodeRuneInString(src[i:])
				i += w
			}
			emit(token{kind: tokIdent, text: src[start:i], pos: start})
		default:
			return nil, &SyntaxError{Pos: i, Msg: "unexpected character " + strconv.QuoteRune(rune(c))}
		}
	}
	endLine(len(src))
	emit(token{kind: tokEOF, pos: len(src)})
	return toks, nil
}

func lexInt(src string, i int) (token, int, error) {
	j := i
	if src[j] == '-' {
		j++
	}
	start := j
	for j < len(src) && src[j] >= '0' && src[j] <= '9' {
		j++
	}
	if j == start {
		return token{}, 0, &SyntaxError{Pos: i, Msg: "expected digits"}
	}
	n, err := strconv.ParseInt(src[i:j], 10, 64)
	if err != nil {
		return token{}, 0, &SyntaxError{Pos: i, Msg: "integer out of range"}
	}
	return token{kind: tokInt, text: src[i:j], num: n, pos: i}, j, nil
}

func isIdentStart(src string, i int) bool {
	r, _ := utf8.DecodeRuneInString(src[i:])
	return r == '_' || unicode.IsLetter(r)
}

func isIdentPart(src string, i int) bool {
	r, _ := utf8.DecodeRuneInString(src[i:])
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

type parser struct {
	toks  []token
	i     int
	store *Store
}

func (p *parser) peek() token { return p.toks[p.i] }

// skipLines steps over statement terminators, at the points where a new
// statement may begin.
func (p *parser) skipLines() {
	for p.at(tokNewline) {
		p.i++
	}
}
func (p *parser) next() token         { t := p.toks[p.i]; p.i++; return t }
func (p *parser) at(k tokenKind) bool { return p.toks[p.i].kind == k }

func (p *parser) expect(k tokenKind, what string) (token, error) {
	if !p.at(k) {
		return token{}, p.unexpected("expected " + what + ", found ")
	}
	return p.next(), nil
}

// unexpected reports the token ahead, marking the error incomplete when the
// input simply ran out.
func (p *parser) unexpected(prefix string) *SyntaxError {
	t := p.peek()
	return &SyntaxError{Pos: t.pos, Msg: prefix + t.describe(), Incomplete: t.kind == tokEOF}
}

// continuesLine reports whether the token ahead sits on the same line as the
// one behind it. An application suffix may only extend a term it shares a line
// with, so a statement beginning with "(" starts a new statement instead of
// silently becoming an argument to the line above.
func (p *parser) continuesLine() bool {
	return p.i > 0 && p.toks[p.i].line == p.toks[p.i-1].line
}

func (p *parser) term(depth int) (Term, error) {
	if depth > maxNesting {
		return nil, &SyntaxError{Pos: p.peek().pos, Msg: "nested too deeply"}
	}
	if p.at(tokIdent) && p.toks[p.i+1].kind == tokArrow {
		param := p.next().text
		p.next() // "=>"
		p.skipLines()
		body, err := p.term(depth + 1)
		if err != nil {
			return nil, err
		}
		return TermLambda{Param: param, Body: body}, nil
	}
	return p.apply(depth)
}

func (p *parser) apply(depth int) (Term, error) {
	t, err := p.primary(depth)
	if err != nil {
		return nil, err
	}
	for p.at(tokOpen) && p.continuesLine() {
		p.next()
		arg, err := p.term(depth + 1)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokClose, `")"`); err != nil {
			return nil, err
		}
		t = TermApply{Fn: t, Arg: arg}
	}
	return t, nil
}

func (p *parser) primary(depth int) (Term, error) {
	switch t := p.peek(); t.kind {
	case tokOpen:
		p.next()
		inner, err := p.term(depth + 1)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokClose, `")"`); err != nil {
			return nil, err
		}
		return inner, nil
	case tokIdent:
		p.next()
		return TermName{Name: t.text}, nil
	case tokInt:
		p.next()
		return TermAtom{Payload: IntAtom(t.num)}, nil
	case tokText:
		p.next()
		return TermAtom{Payload: TextAtom(t.text)}, nil
	}
	return nil, p.unexpected("expected a term, found ")
}

func (p *parser) pattern(depth int) (Pattern, error) {
	if depth > maxNesting {
		return nil, &SyntaxError{Pos: p.peek().pos, Msg: "nested too deeply"}
	}
	pat, err := p.patternPrimary(depth)
	if err != nil {
		return nil, err
	}
	for p.at(tokOpen) && p.continuesLine() {
		p.next()
		arg, err := p.pattern(depth + 1)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokClose, `")"`); err != nil {
			return nil, err
		}
		pat = PApply{Fn: pat, Arg: arg}
	}
	return pat, nil
}

func (p *parser) patternPrimary(depth int) (Pattern, error) {
	switch t := p.peek(); t.kind {
	case tokQuery:
		p.next()
		name, err := p.expect(tokIdent, "a capture name")
		if err != nil {
			return nil, err
		}
		return PCapture{Name: name.text}, nil
	case tokOpen:
		p.next()
		inner, err := p.pattern(depth + 1)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokClose, `")"`); err != nil {
			return nil, err
		}
		return inner, nil
	case tokIdent:
		p.next()
		if t.text == "_" {
			return PWildcard{}, nil
		}
		return PConst{Value: p.store.Atom(NameAtom(t.text))}, nil
	case tokInt:
		p.next()
		return PConst{Value: p.store.Atom(IntAtom(t.num))}, nil
	case tokText:
		p.next()
		return PConst{Value: p.store.Atom(TextAtom(t.text))}, nil
	}
	return nil, p.unexpected("expected a pattern, found ")
}

// ParseProgram reads a whole Loom program: a sequence of statements.
//
//	identity = x => x
//	friend   = x => y => knows(x)(y)
//	transaction {
//	    assert friend(Alice)(Bob)
//	    assert friend(Bob)(Charlie)
//	}
//	answer = holds(friend(Alice)(Bob))
//
// Statements need no separator. A newline ends a statement that is complete
// there, and is ignored where a term is still required, so a lambda body may sit
// on the line below its "=>". Because an application's "(" only continues a term
// on the same line, a statement beginning with "(" starts a new statement rather
// than feeding an argument to the line above.
//
// Unlike Parse this needs the store, because a match pattern's constants name
// values that must live in one.
func (s *Store) ParseProgram(src string) (*Program, error) {
	p, err := newStatementParser(s, src)
	if err != nil {
		return nil, err
	}
	prog := &Program{}
	for p.skipLines(); !p.at(tokEOF); p.skipLines() {
		c, err := p.statement(0)
		if err != nil {
			return nil, err
		}
		prog.Commands = append(prog.Commands, c)
	}
	return prog, nil
}

// ParseCommand reads exactly one statement, which is what a REPL needs.
func (s *Store) ParseCommand(src string) (Command, error) {
	p, err := newStatementParser(s, src)
	if err != nil {
		return nil, err
	}
	p.skipLines()
	c, err := p.statement(0)
	if err != nil {
		return nil, err
	}
	p.skipLines()
	if !p.at(tokEOF) {
		return nil, &SyntaxError{Pos: p.peek().pos, Msg: "unexpected " + p.peek().describe()}
	}
	return c, nil
}

func newStatementParser(s *Store, src string) (*parser, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	return &parser{toks: toks, store: s}, nil
}

// statement keywords are contextual: assert, retract, match and transaction
// introduce a command only at the start of a statement. Anywhere else they are
// ordinary identifiers, so knows(assert) is still a perfectly good value.
func (p *parser) statement(depth int) (Command, error) {
	if t := p.peek(); t.kind == tokIdent {
		switch t.text {
		case "assert":
			p.next()
			p.skipLines()
			term, err := p.term(depth)
			if err != nil {
				return nil, err
			}
			return Assert{Term: term}, nil
		case "retract":
			p.next()
			p.skipLines()
			claim, err := p.claimRef()
			if err != nil {
				return nil, err
			}
			return Retract{Claim: claim}, nil
		case "match":
			p.next()
			p.skipLines()
			pat, err := p.pattern(depth)
			if err != nil {
				return nil, err
			}
			return Query{Pattern: pat}, nil
		case "transaction":
			p.next()
			return p.transaction(depth)
		}
		if p.toks[p.i+1].kind == tokAssign {
			name := p.next().text
			p.next() // "="
			p.skipLines()
			term, err := p.term(depth)
			if err != nil {
				return nil, err
			}
			return Define{Name: name, Term: term}, nil
		}
	}
	term, err := p.term(depth)
	if err != nil {
		return nil, err
	}
	return Evaluate{Term: term}, nil
}

func (p *parser) transaction(depth int) (Command, error) {
	if depth > maxNesting {
		return nil, &SyntaxError{Pos: p.peek().pos, Msg: "nested too deeply"}
	}
	if _, err := p.expect(tokBraceOpen, `"{"`); err != nil {
		return nil, err
	}
	tx := Transaction{}
	for p.skipLines(); !p.at(tokBraceClose); p.skipLines() {
		if p.at(tokEOF) {
			return nil, &SyntaxError{Pos: p.peek().pos, Msg: `unterminated transaction, expected "}"`, Incomplete: true}
		}
		c, err := p.statement(depth + 1)
		if err != nil {
			return nil, err
		}
		switch c.(type) {
		case Assert, Retract:
		default:
			return nil, &SyntaxError{Pos: p.peek().pos, Msg: "only assert and retract may appear in a transaction"}
		}
		tx.Body = append(tx.Body, c)
	}
	p.next() // "}"
	return tx, nil
}

// claimRef reads a claim identity, written either bare or in the claim:1 form
// that results are printed in.
func (p *parser) claimRef() (ClaimID, error) {
	if p.at(tokIdent) && p.peek().text == "claim" && p.toks[p.i+1].kind == tokColon {
		p.next()
		p.next()
	}
	t, err := p.expect(tokInt, "a claim id")
	if err != nil {
		return 0, err
	}
	if t.num <= 0 {
		return 0, &SyntaxError{Pos: t.pos, Msg: "claim ids start at 1"}
	}
	return ClaimID(t.num), nil
}

// Format renders a term back to surface syntax. It is a debugging aid, not a
// canonical representation: canonicalization is defined for values (§36), not
// for the programs that produce them.
func Format(t Term) string {
	var b strings.Builder
	formatTerm(&b, t)
	return b.String()
}

func formatTerm(b *strings.Builder, t Term) {
	switch x := t.(type) {
	case TermAtom:
		sourceAtom(b, x.Payload)
	case TermName:
		b.WriteString(x.Name)
	case TermVar:
		b.WriteString(x.Name)
	case TermDef:
		b.WriteString(x.Name)
	case TermLambda:
		b.WriteString(x.Param)
		b.WriteString(" => ")
		formatTerm(b, x.Body)
	case TermApply:
		if _, lambda := x.Fn.(TermLambda); lambda {
			b.WriteByte('(')
			formatTerm(b, x.Fn)
			b.WriteByte(')')
		} else {
			formatTerm(b, x.Fn)
		}
		b.WriteByte('(')
		formatTerm(b, x.Arg)
		b.WriteByte(')')
	case nil:
		b.WriteString("<nil>")
	}
}
