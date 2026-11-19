package formula

import "fmt"

type valueType int

const (
	typeNull valueType = iota
	typeNumber
	typeString
	typeBool
	typeList
	typeObject
	typeAny
)

func (t valueType) String() string {
	switch t {
	case typeNull:
		return "空值"
	case typeNumber:
		return "数值"
	case typeString:
		return "字符串"
	case typeBool:
		return "布尔值"
	case typeList:
		return "列表"
	case typeObject:
		return "对象"
	case typeAny:
		return "任意值"
	default:
		return "未知类型"
	}
}

var plainVariables = map[string]valueType{
	"class.totalAverage":        typeNumber,
	"grade.totalAverage":        typeNumber,
	"statGroup.passCount":       typeNumber,
	"statGroup.target":          typeNumber,
	"statGroup.memberCount":     typeNumber,
	"tier.actual":               typeNumber,
	"tier.base":                 typeNumber,
	"program.targetCompletion":  typeNumber,
	"program.undergraduatePass": typeNumber,
}

var indexedVariables = map[string]valueType{
	"class.average":    typeNumber,
	"class.count":      typeNumber,
	"grade.average":    typeNumber,
	"baseline.average": typeNumber,
	"baseline.ratio":   typeNumber,
	"tier.count":       typeNumber,
	"tier.target":      typeNumber,
	"indicator":        typeNumber,
}

var indexVariables = map[string]bool{
	"subject": true,
	"tier":    true,
}

var functions = map[string]functionSpec{
	"min":      {minArgs: 1, check: numericArgs, result: typeNumber},
	"max":      {minArgs: 1, check: numericArgs, result: typeNumber},
	"abs":      {minArgs: 1, maxArgs: 1, check: numericArgs, result: typeNumber},
	"round":    {minArgs: 1, maxArgs: 1, check: numericArgs, result: typeNumber},
	"floor":    {minArgs: 1, maxArgs: 1, check: numericArgs, result: typeNumber},
	"ceil":     {minArgs: 1, maxArgs: 1, check: numericArgs, result: typeNumber},
	"if":       {minArgs: 3, maxArgs: 3, check: checkIf, result: typeAny},
	"coalesce": {minArgs: 1, check: checkCoalesce, result: typeAny},
	"sum":      {minArgs: 1, check: numericArgs, result: typeNumber},
	"avg":      {minArgs: 1, check: numericArgs, result: typeNumber},
	"count":    {minArgs: 1, check: checkCount, result: typeNumber},
}

type functionSpec struct {
	minArgs int
	maxArgs int
	check   func(source string, pos int, name string, args []valueType) (valueType, error)
	result  valueType
}

func validateNode(source string, root node, currentIndicator string) error {
	_, err := checkType(source, root, currentIndicator)
	return err
}

func checkType(source string, n node, currentIndicator string) (valueType, error) {
	switch item := n.(type) {
	case *numberNode:
		return typeNumber, nil
	case *stringNode:
		return typeString, nil
	case *boolNode:
		return typeBool, nil
	case *nullNode:
		return typeNull, nil
	case *pathNode:
		return checkPath(source, item, currentIndicator)
	case *unaryNode:
		operand, err := checkType(source, item.operand, currentIndicator)
		if err != nil {
			return 0, err
		}
		switch item.operator {
		case tokenMinus, tokenPlus:
			if operand != typeNumber {
				return 0, newError(source, item.pos, fmt.Sprintf("一元运算需要数值，得到%s", operand))
			}
			return typeNumber, nil
		case tokenBang:
			if operand != typeBool {
				return 0, newError(source, item.pos, fmt.Sprintf("逻辑非需要布尔值，得到%s", operand))
			}
			return typeBool, nil
		}
	case *binaryNode:
		left, err := checkType(source, item.left, currentIndicator)
		if err != nil {
			return 0, err
		}
		right, err := checkType(source, item.right, currentIndicator)
		if err != nil {
			return 0, err
		}
		switch item.operator {
		case tokenPlus, tokenMinus, tokenStar, tokenSlash, tokenPercent:
			if left != typeNumber || right != typeNumber {
				return 0, newError(source, item.pos, fmt.Sprintf("算术运算需要两个数值，得到%s和%s", left, right))
			}
			if (item.operator == tokenSlash || item.operator == tokenPercent) && isZeroLiteral(item.right) {
				return 0, newError(source, item.pos, "除数不能为字面量零")
			}
			return typeNumber, nil
		case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
			if left != right || (left != typeNumber && left != typeString) {
				return 0, newError(source, item.pos, fmt.Sprintf("比较运算需要同类型的数值或字符串，得到%s和%s", left, right))
			}
			return typeBool, nil
		case tokenEqual, tokenNotEqual:
			if left != right && left != typeNull && right != typeNull {
				return 0, newError(source, item.pos, fmt.Sprintf("相等比较需要同类型的值，得到%s和%s", left, right))
			}
			return typeBool, nil
		case tokenAnd, tokenOr:
			if left != typeBool || right != typeBool {
				return 0, newError(source, item.pos, fmt.Sprintf("逻辑运算需要两个布尔值，得到%s和%s", left, right))
			}
			return typeBool, nil
		}
	case *conditionalNode:
		condition, err := checkType(source, item.condition, currentIndicator)
		if err != nil {
			return 0, err
		}
		if condition != typeBool {
			return 0, newError(source, item.pos, fmt.Sprintf("条件表达式需要布尔条件，得到%s", condition))
		}
		then, err := checkType(source, item.then, currentIndicator)
		if err != nil {
			return 0, err
		}
		otherwise, err := checkType(source, item.otherwise, currentIndicator)
		if err != nil {
			return 0, err
		}
		return mergeTypes(source, item.pos, then, otherwise)
	case *callNode:
		spec, ok := functions[item.name]
		if !ok {
			return 0, newError(source, item.pos, fmt.Sprintf("不允许调用函数 %q", item.name))
		}
		if len(item.args) < spec.minArgs {
			return 0, newError(source, item.pos, fmt.Sprintf("函数 %s 至少需要 %d 个参数", item.name, spec.minArgs))
		}
		if spec.maxArgs > 0 && len(item.args) > spec.maxArgs {
			return 0, newError(source, item.pos, fmt.Sprintf("函数 %s 最多接受 %d 个参数", item.name, spec.maxArgs))
		}
		types := make([]valueType, len(item.args))
		for i, arg := range item.args {
			argType, err := checkType(source, arg, currentIndicator)
			if err != nil {
				return 0, err
			}
			types[i] = argType
		}
		return spec.check(source, item.pos, item.name, types)
	}
	return 0, newError(source, n.Pos(), "不支持的表达式")
}

func checkPath(source string, path *pathNode, currentIndicator string) (valueType, error) {
	if indexVariables[path.root] && len(path.access) == 0 {
		return typeString, nil
	}
	if len(path.access) == 0 {
		if variableType, ok := plainVariables[path.root]; ok {
			return variableType, nil
		}
		return 0, newError(source, path.pos, fmt.Sprintf("未知变量 %q", path.root))
	}
	prefix := path.root
	for i, item := range path.access {
		if item.field != "" {
			if i > 0 {
				return 0, newError(source, path.pos, fmt.Sprintf("变量路径 %s 的字段层级不正确", path.String()))
			}
			prefix = path.root + "." + item.field
		}
		if item.index == nil {
			if i == len(path.access)-1 {
				if variableType, ok := plainVariables[prefix]; ok {
					return variableType, nil
				}
				if _, ok := indexedVariables[prefix]; ok {
					return 0, newError(source, path.pos, fmt.Sprintf("变量 %s 需要索引", prefix))
				}
			}
			return 0, newError(source, path.pos, fmt.Sprintf("未知变量 %q", prefix))
		}
		if i != len(path.access)-1 {
			return 0, newError(source, path.pos, fmt.Sprintf("变量 %s 的索引位置不正确", path.String()))
		}
		if _, ok := indexedVariables[prefix]; !ok {
			return 0, newError(source, path.pos, fmt.Sprintf("未知变量 %q", prefix))
		}
		if err := checkIndex(source, prefix, item.index, currentIndicator); err != nil {
			return 0, err
		}
		return indexedVariables[prefix], nil
	}
	return 0, newError(source, path.pos, fmt.Sprintf("未知变量 %q", path.String()))
}

func checkIndex(source, prefix string, index node, currentIndicator string) error {
	switch item := index.(type) {
	case *stringNode:
		if prefix == "indicator" && item.value == currentIndicator {
			return newError(source, item.pos, fmt.Sprintf("指标 %s 不能引用自身", currentIndicator))
		}
		return nil
	case *pathNode:
		if len(item.access) == 0 && indexVariables[item.root] {
			return nil
		}
		return newError(source, item.pos, fmt.Sprintf("索引 %s 必须是字符串或者 subject、tier", item.String()))
	default:
		return newError(source, index.Pos(), "索引必须是字符串或者 subject、tier")
	}
}

func mergeTypes(source string, pos int, left, right valueType) (valueType, error) {
	if left == typeNull {
		return right, nil
	}
	if right == typeNull {
		return left, nil
	}
	if left != right {
		return 0, newError(source, pos, fmt.Sprintf("条件分支类型不一致，得到%s和%s", left, right))
	}
	return left, nil
}

func numericArgs(source string, pos int, name string, args []valueType) (valueType, error) {
	for _, arg := range args {
		if arg == typeList {
			continue
		}
		if arg != typeNumber {
			return 0, newError(source, pos, fmt.Sprintf("函数 %s 需要数值参数，得到%s", name, arg))
		}
	}
	return typeNumber, nil
}

func checkIf(source string, pos int, name string, args []valueType) (valueType, error) {
	if args[0] != typeBool {
		return 0, newError(source, pos, fmt.Sprintf("函数 %s 的第一个参数必须是布尔值，得到%s", name, args[0]))
	}
	return mergeTypes(source, pos, args[1], args[2])
}

func checkCoalesce(source string, pos int, name string, args []valueType) (valueType, error) {
	result := typeNull
	for _, arg := range args {
		if arg == typeNull {
			continue
		}
		if result == typeNull {
			result = arg
			continue
		}
		if arg != result {
			return 0, newError(source, pos, fmt.Sprintf("函数 %s 的参数类型不一致，得到%s和%s", name, result, arg))
		}
	}
	return result, nil
}

func checkCount(source string, pos int, name string, args []valueType) (valueType, error) {
	return typeNumber, nil
}

func isZeroLiteral(n node) bool {
	value, ok := n.(*numberNode)
	return ok && value.value == 0
}
