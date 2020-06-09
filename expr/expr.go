package expr

import (
	"errors"
	"fmt"
	"regexp/syntax"
	"strings"
	"unicode/utf8"
)

type exprPart struct {
	neg   bool
	code  string
	value string
}

func runeLen(b byte) int {
	return int([16]byte{
		// ASCII Range:
		1, 1, 1, 1, 1, 1, 1, 1,
		// continuation, then multi byte.
		1, 1, 1, 1, 2, 2, 3, 4,
	}[b>>4])
}

func consumeCharClass(expr string) int {
	pos := 1

	if pos < len(expr) && expr[pos] == '^' {
		pos++
	}

	// First character can be a `]` without needing to be escaped.
	if pos < len(expr) && expr[pos] == ']' {
		pos++
	}

	for pos < len(expr) {
		switch expr[pos] {
		case '\\':
			pos++
			if pos < len(expr) {
				pos += runeLen(expr[pos])
			}

		case '[':
			pos++
			// If this is something like `[:alpha:]` jump to the end.
			if pos < len(expr) && expr[pos] == ':' {
				pos++
				end := strings.IndexByte(expr[pos:], ']') + pos
				if end < pos {
					return -1
				}
				pos = end
			}

		case ']':
			return pos + 1

		default:
			pos += runeLen(expr[pos])
		}
	}

	return -1 // Ran off the end
}

func consumeQE(expr string) int {
	pos := 1
	width := strings.Index(expr[pos:], `\E`)
	if width < 0 {
		return width
	}
	return pos + width + 2
}

func consumeParen(expr string, depth int) int {
	if depth > 100 {
		return -1
	}

	pos := 1

	for pos < len(expr) {
		switch expr[pos] {
		case ')':
			return pos + 1

		case '\\':
			pos++
			switch r, size := utf8.DecodeRuneInString(expr[pos:]); r {
			case 'Q':
				width := consumeQE(expr[pos:])
				if width < 0 {
					return width
				}
				pos += width

			default:
				pos += size
			}

		case '[':
			width := consumeCharClass(expr[pos:])
			if width < 0 {
				return width
			}
			pos += width

		case '(':
			width := consumeParen(expr[pos:], depth+1)
			if width < 0 {
				return width
			}
			pos += width

		default:
			pos += runeLen(expr[pos])
		}
	}

	return -1
}

func consumeString(expr string) int {
	pos := 1
	for pos < len(expr) {
		switch expr[pos] {
		case '"':
			return pos + 1

		case '\\':
			pos++
			if pos < len(expr) {
				pos += runeLen(expr[pos])
			}

		default:
			pos += runeLen(expr[pos])
		}
	}

	return -1
}

func cleanRegexp(re *syntax.Regexp) {
	for _, s := range re.Sub {
		cleanRegexp(s)
	}

	switch re.Op {
	case syntax.OpCapture:
		*re = *re.Sub[0]
	case syntax.OpConcat:
		i := 0
		for _, s := range re.Sub {
			if s.Op != syntax.OpEmptyMatch {
				re.Sub[i] = s
				i++
			}
		}
		if i == 0 {
			re.Op = syntax.OpEmptyMatch
		} else {
			re.Sub = re.Sub[:i]
		}
	}
}

func Parse(expr string) (*Expression, error) {
	var parts []*ExpressionPart

	start := 0
	pos := 0

	var cur *ExpressionPart

	placeValue := func(v string) error {
		if cur == nil {
			cur = &ExpressionPart{}
		}
		if cur.Code == ExpressionPart_SNIPPET {
			cur.Negated = v[0] == '-'
			if cur.Negated {
				cur.Code = ExpressionPart_MATCH
				v = v[1:]
			}
		}

		re, err := syntax.Parse(v, syntax.Perl)
		if err != nil {
			return fmt.Errorf("around position %d problem parsing regular expression: %v", pos, err)
		}

		// Remove all capturing groups - we don't use them, so no need to keep
		// them. Attempt to remove EmptyMatch expressions where it makes sense.
		cleanRegexp(re)

		if re.Op == syntax.OpEmptyMatch || re.Op == syntax.OpNoMatch {
			return fmt.Errorf("expression around %d to %d is empty", start, pos)
		}

		cur.Expression = re.String()
		parts = append(parts, cur)
		cur = nil
		return nil
	}

	for pos < len(expr) {
		switch expr[pos] {
		case '+':
			atStart := start == pos
			pos++
			if atStart {
				cur = &ExpressionPart{
					Code: ExpressionPart_MATCH,
				}
				start = pos
			}

		case ':':
			neg := expr[start] == '-'
			code := expr[start:pos]
			if neg {
				code = code[1:]
			}
			pos++

			switch code {
			case "f", "file":
				cur = &ExpressionPart{
					Negated: neg,
					Code:    ExpressionPart_FILE,
				}
				start = pos

			case "lang":
				cur = &ExpressionPart{
					Negated: neg,
					Code:    ExpressionPart_LANGUAGE,
				}
				start = pos
			}

		case ' ':
			if start == pos {
				if cur != nil {
					return nil, fmt.Errorf("%v followed by empty expression", cur.GetCode())
				}
			} else {
				if err := placeValue(expr[start:pos]); err != nil {
					return nil, err
				}
			}

			pos++
			start = pos

		case '\\':
			pos++
			switch r, size := utf8.DecodeRuneInString(expr[pos:]); r {
			case 'Q':
				width := consumeQE(expr[pos:])
				if width < 0 {
					return nil, fmt.Errorf("unmatched \\Q at %d", pos)
				}
				pos += width

			default:
				pos += size
			}

		case '[':
			width := consumeCharClass(expr[pos:])
			if width < 0 {
				return nil, fmt.Errorf("invalid character class at position %d", pos)
			}
			pos += width

		case '(':
			width := consumeParen(expr[pos:], 0)
			if width < 0 {
				return nil, fmt.Errorf("invalid parens at position %d", pos)
			}
			pos += width

		default:
			pos += runeLen(expr[pos])
		}
	}

	if start != pos {
		if err := placeValue(expr[start:pos]); err != nil {
			return nil, err
		}
	}

	if len(parts) == 0 {
		return nil, errors.New("no expression found")
	}

	return &Expression{
		Parts: parts,
	}, nil
}
