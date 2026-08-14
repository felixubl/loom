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
// An identifier is a variable when an enclosing lambda binds it and an atom
// otherwise, which is why knows(Alice) is two atoms while x => x is a lambda
// over a variable. Parse is store-free: it produces syntax, not values.
func Parse(src string) (Term, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	t, err := p.term(0)
	if err != nil {
		return nil, err
	}
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
	pat, err := p.pattern(0)
	if err != nil {
		return nil, err
	}
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
)

type token struct {
	kind tokenKind
	text string
	num  int64
	pos  int
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
	}
	return strconv.Quote(t.text)
}

func lex(src string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, token{kind: tokOpen, pos: i})
			i++
		case c == ')':
			toks = append(toks, token{kind: tokClose, pos: i})
			i++
		case c == '?':
			toks = append(toks, token{kind: tokQuery, pos: i})
			i++
		case c == '=' && i+1 < len(src) && src[i+1] == '>':
			toks = append(toks, token{kind: tokArrow, pos: i})
			i += 2
		case c == '"':
			text, next, err := lexText(src, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{kind: tokText, text: text, pos: i})
			i = next
		case c == '-' || (c >= '0' && c <= '9'):
			tok, next, err := lexInt(src, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, tok)
			i = next
		case isIdentStart(src, i):
			start := i
			for i < len(src) && isIdentPart(src, i) {
				_, w := utf8.DecodeRuneInString(src[i:])
				i += w
			}
			toks = append(toks, token{kind: tokIdent, text: src[start:i], pos: start})
		default:
			return nil, &SyntaxError{Pos: i, Msg: "unexpected character " + strconv.QuoteRune(rune(c))}
		}
	}
	return append(toks, token{kind: tokEOF, pos: len(src)}), nil
}

func lexText(src string, i int) (string, int, error) {
	for j := i + 1; j < len(src); j++ {
		if src[j] == '\\' {
			j++
			continue
		}
		if src[j] == '"' {
			text, err := strconv.Unquote(src[i : j+1])
			if err != nil {
				return "", 0, &SyntaxError{Pos: i, Msg: "malformed string literal"}
			}
			return text, j + 1, nil
		}
	}
	return "", 0, &SyntaxError{Pos: i, Msg: "unterminated string literal"}
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
	bound []string
	store *Store
}

func (p *parser) peek() token         { return p.toks[p.i] }
func (p *parser) next() token         { t := p.toks[p.i]; p.i++; return t }
func (p *parser) at(k tokenKind) bool { return p.toks[p.i].kind == k }

func (p *parser) expect(k tokenKind, what string) (token, error) {
	if !p.at(k) {
		return token{}, &SyntaxError{Pos: p.peek().pos, Msg: "expected " + what + ", found " + p.peek().describe()}
	}
	return p.next(), nil
}

func (p *parser) isBound(name string) bool {
	for i := len(p.bound) - 1; i >= 0; i-- {
		if p.bound[i] == name {
			return true
		}
	}
	return false
}

func (p *parser) term(depth int) (Term, error) {
	if depth > maxNesting {
		return nil, &SyntaxError{Pos: p.peek().pos, Msg: "nested too deeply"}
	}
	if p.at(tokIdent) && p.toks[p.i+1].kind == tokArrow {
		param := p.next().text
		p.next() // "=>"
		p.bound = append(p.bound, param)
		body, err := p.term(depth + 1)
		p.bound = p.bound[:len(p.bound)-1]
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
	for p.at(tokOpen) {
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
		if p.isBound(t.text) {
			return TermVar{Name: t.text}, nil
		}
		return TermAtom{Payload: NameAtom(t.text)}, nil
	case tokInt:
		p.next()
		return TermAtom{Payload: IntAtom(t.num)}, nil
	case tokText:
		p.next()
		return TermAtom{Payload: TextAtom(t.text)}, nil
	}
	return nil, &SyntaxError{Pos: p.peek().pos, Msg: "expected a term, found " + p.peek().describe()}
}

func (p *parser) pattern(depth int) (Pattern, error) {
	if depth > maxNesting {
		return nil, &SyntaxError{Pos: p.peek().pos, Msg: "nested too deeply"}
	}
	pat, err := p.patternPrimary(depth)
	if err != nil {
		return nil, err
	}
	for p.at(tokOpen) {
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
	return nil, &SyntaxError{Pos: p.peek().pos, Msg: "expected a pattern, found " + p.peek().describe()}
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
	case TermVar:
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
