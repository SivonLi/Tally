package formula

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenNumber
	tokenString
	tokenIdent
	tokenDot
	tokenLBracket
	tokenRBracket
	tokenLParen
	tokenRParen
	tokenComma
	tokenPlus
	tokenMinus
	tokenStar
	tokenSlash
	tokenPercent
	tokenBang
	tokenLess
	tokenLessEqual
	tokenGreater
	tokenGreaterEqual
	tokenEqual
	tokenNotEqual
	tokenAnd
	tokenOr
	tokenQuestion
	tokenColon
)

type token struct {
	kind tokenKind
	text string
	pos  int
}

type lexer struct {
	source string
	offset int
}

func newLexer(source string) *lexer {
	return &lexer{source: source}
}

func (l *lexer) next() (token, error) {
	for l.offset < len(l.source) {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if !unicode.IsSpace(r) {
			break
		}
		l.offset += size
	}
	if l.offset >= len(l.source) {
		return token{kind: tokenEOF, pos: l.offset}, nil
	}
	start := l.offset
	r, size := utf8.DecodeRuneInString(l.source[l.offset:])
	l.offset += size

	switch r {
	case '.':
		if l.offset < len(l.source) {
			next, _ := utf8.DecodeRuneInString(l.source[l.offset:])
			if unicode.IsDigit(next) {
				return l.number(start)
			}
		}
		return token{kind: tokenDot, text: ".", pos: start}, nil
	case '[':
		return token{kind: tokenLBracket, text: "[", pos: start}, nil
	case ']':
		return token{kind: tokenRBracket, text: "]", pos: start}, nil
	case '(':
		return token{kind: tokenLParen, text: "(", pos: start}, nil
	case ')':
		return token{kind: tokenRParen, text: ")", pos: start}, nil
	case ',':
		return token{kind: tokenComma, text: ",", pos: start}, nil
	case '+':
		return token{kind: tokenPlus, text: "+", pos: start}, nil
	case '-':
		return token{kind: tokenMinus, text: "-", pos: start}, nil
	case '*':
		return token{kind: tokenStar, text: "*", pos: start}, nil
	case '/':
		return token{kind: tokenSlash, text: "/", pos: start}, nil
	case '%':
		return token{kind: tokenPercent, text: "%", pos: start}, nil
	case '?':
		return token{kind: tokenQuestion, text: "?", pos: start}, nil
	case ':':
		return token{kind: tokenColon, text: ":", pos: start}, nil
	case '!':
		if l.peek('=') {
			l.offset++
			return token{kind: tokenNotEqual, text: "!=", pos: start}, nil
		}
		return token{kind: tokenBang, text: "!", pos: start}, nil
	case '<':
		if l.peek('=') {
			l.offset++
			return token{kind: tokenLessEqual, text: "<=", pos: start}, nil
		}
		return token{kind: tokenLess, text: "<", pos: start}, nil
	case '>':
		if l.peek('=') {
			l.offset++
			return token{kind: tokenGreaterEqual, text: ">=", pos: start}, nil
		}
		return token{kind: tokenGreater, text: ">", pos: start}, nil
	case '=':
		if l.peek('=') {
			l.offset++
			return token{kind: tokenEqual, text: "==", pos: start}, nil
		}
		return token{}, l.errorAt(start, "单个 = 不能用于比较，请使用 ==")
	case '&':
		if l.peek('&') {
			l.offset++
			return token{kind: tokenAnd, text: "&&", pos: start}, nil
		}
		return token{}, l.errorAt(start, "单个 & 不能用于逻辑运算，请使用 &&")
	case '|':
		if l.peek('|') {
			l.offset++
			return token{kind: tokenOr, text: "||", pos: start}, nil
		}
		return token{}, l.errorAt(start, "单个 | 不能用于逻辑运算，请使用 ||")
	case '\'', '"':
		return l.stringLiteral(start, r)
	}

	if unicode.IsDigit(r) {
		return l.number(start)
	}
	if unicode.IsLetter(r) || r == '_' {
		return l.identifier(start), nil
	}
	return token{}, l.errorAt(start, fmt.Sprintf("无法识别的字符 %q", r))
}

func (l *lexer) peek(want rune) bool {
	if l.offset >= len(l.source) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(l.source[l.offset:])
	return r == want
}

func (l *lexer) number(start int) (token, error) {
	seenDot := false
	for l.offset < len(l.source) {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if unicode.IsDigit(r) {
			l.offset += size
			continue
		}
		if r == '.' && !seenDot {
			seenDot = true
			l.offset += size
			continue
		}
		break
	}
	if l.offset < len(l.source) {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if r == 'e' || r == 'E' {
			l.offset += size
			if l.offset < len(l.source) {
				sign, signSize := utf8.DecodeRuneInString(l.source[l.offset:])
				if sign == '+' || sign == '-' {
					l.offset += signSize
				}
			}
			digits := l.offset
			for l.offset < len(l.source) {
				digit, digitSize := utf8.DecodeRuneInString(l.source[l.offset:])
				if !unicode.IsDigit(digit) {
					break
				}
				l.offset += digitSize
			}
			if digits == l.offset {
				return token{}, l.errorAt(start, "指数缺少数字")
			}
		}
	}
	text := l.source[start:l.offset]
	if _, err := strconv.ParseFloat(text, 64); err != nil {
		return token{}, l.errorAt(start, "数值格式不正确")
	}
	return token{kind: tokenNumber, text: text, pos: start}, nil
}

func (l *lexer) stringLiteral(start int, quote rune) (token, error) {
	var builder strings.Builder
	for l.offset < len(l.source) {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		l.offset += size
		if r == quote {
			return token{kind: tokenString, text: builder.String(), pos: start}, nil
		}
		if r != '\\' {
			builder.WriteRune(r)
			continue
		}
		if l.offset >= len(l.source) {
			return token{}, l.errorAt(start, "字符串没有结束")
		}
		escaped, escapedSize := utf8.DecodeRuneInString(l.source[l.offset:])
		l.offset += escapedSize
		switch escaped {
		case 'n':
			builder.WriteByte('\n')
		case 't':
			builder.WriteByte('\t')
		case '\\':
			builder.WriteByte('\\')
		case '\'':
			builder.WriteByte('\'')
		case '"':
			builder.WriteByte('"')
		default:
			return token{}, l.errorAt(start, fmt.Sprintf("不支持的转义 %q", escaped))
		}
	}
	return token{}, l.errorAt(start, "字符串没有结束")
}

func (l *lexer) identifier(start int) token {
	for l.offset < len(l.source) {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		l.offset += size
	}
	return token{kind: tokenIdent, text: l.source[start:l.offset], pos: start}
}

func (l *lexer) errorAt(pos int, message string) error {
	return newError(l.source, pos, message)
}
