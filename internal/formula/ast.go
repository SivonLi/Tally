package formula

import (
	"fmt"
	"strings"
)

// Error 是带源码位置的表达式错误。
type Error struct {
	Source  string
	Pos     int
	Message string
}

// Error 返回可直接显示的表达式错误。
func (e *Error) Error() string {
	line, column := position(e.Source, e.Pos)
	return fmt.Sprintf("第 %d 行第 %d 列: %s", line, column, e.Message)
}

func newError(source string, pos int, message string) *Error {
	return &Error{Source: source, Pos: pos, Message: message}
}

func position(source string, pos int) (int, int) {
	if pos > len(source) {
		pos = len(source)
	}
	line := 1
	column := 1
	for _, r := range source[:pos] {
		if r == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}

type node interface {
	Pos() int
	Count() int
}

type numberNode struct {
	pos   int
	value float64
}

func (n *numberNode) Pos() int   { return n.pos }
func (n *numberNode) Count() int { return 1 }

type stringNode struct {
	pos   int
	value string
}

func (n *stringNode) Pos() int   { return n.pos }
func (n *stringNode) Count() int { return 1 }

type boolNode struct {
	pos   int
	value bool
}

func (n *boolNode) Pos() int   { return n.pos }
func (n *boolNode) Count() int { return 1 }

type nullNode struct {
	pos int
}

func (n *nullNode) Pos() int   { return n.pos }
func (n *nullNode) Count() int { return 1 }

type access struct {
	field string
	index node
}

type pathNode struct {
	pos    int
	root   string
	access []access
}

func (n *pathNode) Pos() int { return n.pos }

func (n *pathNode) Count() int {
	count := 1
	for _, item := range n.access {
		if item.index != nil {
			count += item.index.Count()
		}
	}
	return count
}

func (n *pathNode) String() string {
	var builder strings.Builder
	builder.WriteString(n.root)
	for _, item := range n.access {
		if item.field != "" {
			builder.WriteByte('.')
			builder.WriteString(item.field)
		}
		if item.index != nil {
			builder.WriteString("[...]")
		}
	}
	return builder.String()
}

type unaryNode struct {
	pos      int
	operator tokenKind
	operand  node
}

func (n *unaryNode) Pos() int   { return n.pos }
func (n *unaryNode) Count() int { return 1 + n.operand.Count() }

type binaryNode struct {
	pos      int
	operator tokenKind
	left     node
	right    node
}

func (n *binaryNode) Pos() int   { return n.pos }
func (n *binaryNode) Count() int { return 1 + n.left.Count() + n.right.Count() }

type conditionalNode struct {
	pos       int
	condition node
	then      node
	otherwise node
}

func (n *conditionalNode) Pos() int { return n.pos }

func (n *conditionalNode) Count() int {
	return 1 + n.condition.Count() + n.then.Count() + n.otherwise.Count()
}

type callNode struct {
	pos  int
	name string
	args []node
}

func (n *callNode) Pos() int { return n.pos }

func (n *callNode) Count() int {
	count := 1
	for _, arg := range n.args {
		count += arg.Count()
	}
	return count
}
