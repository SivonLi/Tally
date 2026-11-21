package formula

import "fmt"

// IndicatorReferences 返回表达式引用的其他指标键。
func (e *Expression) IndicatorReferences() ([]string, error) {
	var references []string
	err := walkReferences(e.source, e.root, &references)
	return references, err
}

func walkReferences(source string, n node, references *[]string) error {
	switch item := n.(type) {
	case *pathNode:
		for _, access := range item.access {
			if access.index == nil {
				continue
			}
			if item.root == "indicator" {
				literal, ok := access.index.(*stringNode)
				if !ok {
					return newError(source, access.index.Pos(), "indicator 的索引必须是字符串常量")
				}
				*references = append(*references, literal.value)
			}
			if err := walkReferences(source, access.index, references); err != nil {
				return err
			}
		}
	case *unaryNode:
		return walkReferences(source, item.operand, references)
	case *binaryNode:
		if err := walkReferences(source, item.left, references); err != nil {
			return err
		}
		return walkReferences(source, item.right, references)
	case *conditionalNode:
		if err := walkReferences(source, item.condition, references); err != nil {
			return err
		}
		if err := walkReferences(source, item.then, references); err != nil {
			return err
		}
		return walkReferences(source, item.otherwise, references)
	case *callNode:
		for _, arg := range item.args {
			if err := walkReferences(source, arg, references); err != nil {
				return err
			}
		}
	case *numberNode, *stringNode, *boolNode, *nullNode:
		return nil
	default:
		return fmt.Errorf("不支持的表达式节点")
	}
	return nil
}
