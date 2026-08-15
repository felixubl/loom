package loom

import "strings"

// Text escaping, specified so that two independent implementations produce the
// same bytes. See docs/loom-canonical-form.md.
//
// Only what must be escaped is escaped:
//
//	"     → \"
//	\     → \\
//	\n    → \n        (0x0A)
//	\r    → \r        (0x0D)
//	\t    → \t        (0x09)
//	other bytes < 0x20, and 0x7F → \u00xx, lowercase hex
//	everything else → the literal byte
//
// Bytes above 0x7F pass through untouched, so a payload that is not valid UTF-8
// still survives a round trip byte for byte.

const hexDigits = "0123456789abcdef"

func quoteText(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20 || c == 0x7f:
			b.WriteString(`\u00`)
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0xf])
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// unquoteText reads a quoted literal starting at src[i], which must be the
// opening quote. It returns the text and the offset just past the closing
// quote.
//
// \uXXXX is accepted for any code point and decoded to UTF-8, even though
// quoteText only ever emits it for control bytes. Being able to read a form we
// never write costs nothing and makes hand-written source forgiving.
func unquoteText(src string, i int) (string, int, error) {
	var b strings.Builder
	for j := i + 1; j < len(src); j++ {
		c := src[j]
		switch c {
		case '"':
			return b.String(), j + 1, nil
		case '\\':
			j++
			if j >= len(src) {
				return "", 0, &SyntaxError{Pos: i, Msg: "unterminated string literal"}
			}
			switch src[j] {
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				if j+4 >= len(src) {
					return "", 0, &SyntaxError{Pos: j, Msg: `\u needs four hex digits`}
				}
				r, err := hexRune(src[j+1 : j+5])
				if err != nil {
					return "", 0, &SyntaxError{Pos: j, Msg: `\u needs four hex digits`}
				}
				b.WriteRune(r)
				j += 4
			default:
				return "", 0, &SyntaxError{Pos: j, Msg: "unknown escape \\" + string(src[j])}
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, &SyntaxError{Pos: i, Msg: "unterminated string literal"}
}

func hexRune(s string) (rune, error) {
	var r rune
	for i := 0; i < len(s); i++ {
		var d rune
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			d = rune(c - '0')
		case c >= 'a' && c <= 'f':
			d = rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = rune(c-'A') + 10
		default:
			return 0, &SyntaxError{Msg: "not a hex digit"}
		}
		r = r<<4 | d
	}
	return r, nil
}
