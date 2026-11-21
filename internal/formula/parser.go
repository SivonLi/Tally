package formula

import (
	"fmt"
	"strconv"
)

type parser struct {
	source string
	lexer  *lexer
	token  token
}

// Options 控制表达式的编译校验。
type Options struct {
	CurrentIndicator string
	MaxNodes         int
}

// Compile 解析并校验一个受限表达式。
func Compile(source string, options Options) (*Expression, error) {
	p := &parser{source: source, lexer: newLexer(source)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	root, err := p.parseConditional()
	if err != nil {
		return nil, err
	}
	if p.token.kind != tokenEOF {
		return nil, newError(source, p.token.pos, fmt.Sprintf("多余的内容 %q", p.token.text))
	}
	maxNodes := options.MaxNodes
	if maxNodes <= 0 {
		maxNodes = 1000
	}
	if root.Count() > maxNodes {
		return nil, newError(source, root.Pos(), fmt.Sprintf("表达式节点数量超过上限 %d", maxNodes))
	}
	if err := validateNode(source, root, options.CurrentIndicator); err != nil {
		return nil, err
	}
	return &Expression{source: source, root: root}, nil
}

// Expression 是编译后的表达式。
type Expression struct {
	source string
	root   node
}

// Source 返回原始表达式。
func (e *Expression) Source() string {
	return e.source
}

// Eval 在给定环境中计算表达式。
func (e *Expression) Eval(env Env) (Value, error) {
	return evalNode(e.source, e.root, env)
}

func (p *parser) advance() error {
	next, err := p.lexer.next()
	if err != nil {
		return err
	}
	p.token = next
	return nil
}

func (p *parser) expect(kind tokenKind, description string) error {
	if p.token.kind != kind {
		return newError(p.source, p.token.pos, "需要 "+description)
	}
	return p.advance()
}

func (p *parser) parseConditional() (node, error) {
	condition, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.token.kind != tokenQuestion {
		return condition, nil
	}
	pos := p.token.pos
	if err := p.advance(); err != nil {
		return nil, err
	}
	then, err := p.parseConditional()
	if err != nil {
		return nil, err
	}
	if err := p.expect(tokenColon, "条件表达式中的 :"); err != nil {
		return nil, err
	}
	otherwise, err := p.parseConditional()
	if err != nil {
		return nil, err
	}
	return &conditionalNode{pos: pos, condition: condition, then: then, otherwise: otherwise}, nil
}

func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.token.kind == tokenOr {
		pos := p.token.pos
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{pos: pos, operator: tokenOr, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (node, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.token.kind == tokenAnd {
		pos := p.token.pos
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{pos: pos, operator: tokenAnd, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseEquality() (node, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.token.kind == tokenEqual || p.token.kind == tokenNotEqual {
		operator := p.token.kind
		pos := p.token.pos
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{pos: pos, operator: operator, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseComparison() (node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for {
		switch p.token.kind {
		case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		default:
			return left, nil
		}
		operator := p.token.kind
		pos := p.token.pos
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{pos: pos, operator: operator, left: left, right: right}
	}
}

func (p *parser) parseAdditive() (node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.token.kind == tokenPlus || p.token.kind == tokenMinus {
		operator := p.token.kind
		pos := p.token.pos
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{pos: pos, operator: operator, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseMultiplicative() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.token.kind {
		case tokenStar, tokenSlash, tokenPercent:
		default:
			return left, nil
		}
		operator := p.token.kind
		pos := p.token.pos
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{pos: pos, operator: operator, left: left, right: right}
	}
}

func (p *parser) parseUnary() (node, error) {
	switch p.token.kind {
	case tokenMinus, tokenPlus, tokenBang:
		operator := p.token.kind
		pos := p.token.pos
		if err := p.advance(); err != nil {
			return nil, err
		}
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unaryNode{pos: pos, operator: operator, operand: operand}, nil
	default:
		return p.parsePrimary()
	}
}

func (p *parser) parsePrimary() (node, error) {
	switch p.token.kind {
	case tokenNumber:
		value, err := strconv.ParseFloat(p.token.text, 64)
		if err != nil {
			return nil, newError(p.source, p.token.pos, "数值格式不正确")
		}
		pos := p.token.pos
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &numberNode{pos: pos, value: value}, nil
	case tokenString:
		pos := p.token.pos
		value := p.token.text
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &stringNode{pos: pos, value: value}, nil
	case tokenIdent:
		return p.parseIdent()
	case tokenLParen:
		if err := p.advance(); err != nil {
			return nil, err
		}
		inner, err := p.parseConditional()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokenRParen, "右括号"); err != nil {
			return nil, err
		}
		return inner, nil
	case tokenEOF:
		return nil, newError(p.source, p.token.pos, "表达式不完整")
	default:
		return nil, newError(p.source, p.token.pos, "此处需要一个值")
	}
}

func (p *parser) parseIdent() (node, error) {
	pos := p.token.pos
	name := p.token.text
	if err := p.advance(); err != nil {
		return nil, err
	}
	if name == "true" {
		return &boolNode{pos: pos, value: true}, nil
	}
	if name == "false" {
		return &boolNode{pos: pos, value: false}, nil
	}
	if name == "null" {
		return &nullNode{pos: pos}, nil
	}
	if p.token.kind == tokenLParen {
		return p.parseCall(pos, name)
	}
	path := &pathNode{pos: pos, root: name}
	for {
		switch p.token.kind {
		case tokenDot:
			if err := p.advance(); err != nil {
				return nil, err
			}
			if p.token.kind != tokenIdent {
				return nil, newError(p.source, p.token.pos, "点号后面需要字段名")
			}
			field := p.token.text
			if err := p.advance(); err != nil {
				return nil, err
			}
			path.access = append(path.access, access{field: field})
		case tokenLBracket:
			if err := p.advance(); err != nil {
				return nil, err
			}
			index, err := p.parseConditional()
			if err != nil {
				return nil, err
			}
			if err := p.expect(tokenRBracket, "右方括号"); err != nil {
				return nil, err
			}
			if len(path.access) == 0 {
				path.access = append(path.access, access{index: index})
				continue
			}
			last := &path.access[len(path.access)-1]
			if last.index != nil {
				return nil, newError(p.source, p.token.pos, "同一个字段只能有一个索引")
			}
			last.index = index
		default:
			return path, nil
		}
	}
}

func (p *parser) parseCall(pos int, name string) (node, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	var args []node
	if p.token.kind != tokenRParen {
		for {
			arg, err := p.parseConditional()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if p.token.kind != tokenComma {
				break
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
	}
	if err := p.expect(tokenRParen, "右括号"); err != nil {
		return nil, err
	}
	return &callNode{pos: pos, name: name, args: args}, nil
}
