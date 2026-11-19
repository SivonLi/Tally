package formula

import (
	"fmt"
	"math"
)

// Env 是表达式求值时使用的变量集合。
type Env map[string]Value

func evalNode(source string, n node, env Env) (Value, error) {
	switch item := n.(type) {
	case *numberNode:
		return Number(item.value), nil
	case *stringNode:
		return String(item.value), nil
	case *boolNode:
		return Bool(item.value), nil
	case *nullNode:
		return Null(), nil
	case *pathNode:
		return evalPath(source, item, env)
	case *unaryNode:
		operand, err := evalNode(source, item.operand, env)
		if err != nil {
			return Null(), err
		}
		switch item.operator {
		case tokenMinus:
			value, ok := operand.numeric()
			if !ok {
				return Null(), newError(source, item.pos, fmt.Sprintf("一元负号需要数值，得到%s", operand.Format()))
			}
			return Number(-value), nil
		case tokenPlus:
			value, ok := operand.numeric()
			if !ok {
				return Null(), newError(source, item.pos, fmt.Sprintf("一元正号需要数值，得到%s", operand.Format()))
			}
			return Number(value), nil
		case tokenBang:
			value, ok := operand.Bool()
			if !ok {
				return Null(), newError(source, item.pos, fmt.Sprintf("逻辑非需要布尔值，得到%s", operand.Format()))
			}
			return Bool(!value), nil
		}
	case *binaryNode:
		return evalBinary(source, item, env)
	case *conditionalNode:
		condition, err := evalNode(source, item.condition, env)
		if err != nil {
			return Null(), err
		}
		value, ok := condition.Bool()
		if !ok {
			return Null(), newError(source, item.pos, fmt.Sprintf("条件表达式需要布尔条件，得到%s", condition.Format()))
		}
		if value {
			return evalNode(source, item.then, env)
		}
		return evalNode(source, item.otherwise, env)
	case *callNode:
		return evalCall(source, item, env)
	}
	return Null(), newError(source, n.Pos(), "不支持的表达式")
}

func evalPath(source string, path *pathNode, env Env) (Value, error) {
	if indexVariables[path.root] && len(path.access) == 0 {
		value, ok := env[path.root]
		if !ok {
			return Null(), newError(source, path.pos, fmt.Sprintf("变量 %s 缺失", path.root))
		}
		return value, nil
	}
	value, ok := env[path.root]
	if !ok {
		return Null(), newError(source, path.pos, fmt.Sprintf("变量 %s 缺失", path.root))
	}
	prefix := path.root
	for _, item := range path.access {
		if item.field != "" {
			object, ok := value.Object()
			if !ok {
				return Null(), newError(source, path.pos, fmt.Sprintf("变量 %s 不是对象", prefix))
			}
			prefix += "." + item.field
			value, ok = object[item.field]
			if !ok {
				return Null(), newError(source, path.pos, fmt.Sprintf("变量 %s 缺失", prefix))
			}
		}
		if item.index == nil {
			continue
		}
		index, err := evalNode(source, item.index, env)
		if err != nil {
			return Null(), err
		}
		key, ok := index.String()
		if !ok {
			return Null(), newError(source, item.index.Pos(), fmt.Sprintf("索引 %s 必须是字符串", index.Format()))
		}
		object, ok := value.Object()
		if !ok {
			return Null(), newError(source, path.pos, fmt.Sprintf("变量 %s 不是按键取值的对象", prefix))
		}
		value, ok = object[key]
		if !ok {
			return Null(), newError(source, path.pos, fmt.Sprintf("变量 %s[%q] 缺失", prefix, key))
		}
	}
	return value, nil
}

func evalBinary(source string, binary *binaryNode, env Env) (Value, error) {
	if binary.operator == tokenAnd {
		left, err := evalNode(source, binary.left, env)
		if err != nil {
			return Null(), err
		}
		value, ok := left.Bool()
		if !ok {
			return Null(), newError(source, binary.pos, fmt.Sprintf("逻辑运算需要布尔值，得到%s", left.Format()))
		}
		if !value {
			return Bool(false), nil
		}
		right, err := evalNode(source, binary.right, env)
		if err != nil {
			return Null(), err
		}
		rightValue, ok := right.Bool()
		if !ok {
			return Null(), newError(source, binary.pos, fmt.Sprintf("逻辑运算需要布尔值，得到%s", right.Format()))
		}
		return Bool(rightValue), nil
	}
	if binary.operator == tokenOr {
		left, err := evalNode(source, binary.left, env)
		if err != nil {
			return Null(), err
		}
		value, ok := left.Bool()
		if !ok {
			return Null(), newError(source, binary.pos, fmt.Sprintf("逻辑运算需要布尔值，得到%s", left.Format()))
		}
		if value {
			return Bool(true), nil
		}
		right, err := evalNode(source, binary.right, env)
		if err != nil {
			return Null(), err
		}
		rightValue, ok := right.Bool()
		if !ok {
			return Null(), newError(source, binary.pos, fmt.Sprintf("逻辑运算需要布尔值，得到%s", right.Format()))
		}
		return Bool(rightValue), nil
	}

	left, err := evalNode(source, binary.left, env)
	if err != nil {
		return Null(), err
	}
	right, err := evalNode(source, binary.right, env)
	if err != nil {
		return Null(), err
	}
	switch binary.operator {
	case tokenPlus, tokenMinus, tokenStar, tokenSlash, tokenPercent:
		leftValue, ok := left.numeric()
		if !ok {
			return Null(), newError(source, binary.pos, fmt.Sprintf("算术运算需要数值，得到%s", left.Format()))
		}
		rightValue, ok := right.numeric()
		if !ok {
			return Null(), newError(source, binary.pos, fmt.Sprintf("算术运算需要数值，得到%s", right.Format()))
		}
		switch binary.operator {
		case tokenPlus:
			return Number(leftValue + rightValue), nil
		case tokenMinus:
			return Number(leftValue - rightValue), nil
		case tokenStar:
			return Number(leftValue * rightValue), nil
		case tokenSlash:
			if rightValue == 0 {
				return Null(), newError(source, binary.pos, "除数不能为零")
			}
			return Number(leftValue / rightValue), nil
		case tokenPercent:
			if rightValue == 0 {
				return Null(), newError(source, binary.pos, "取余的除数不能为零")
			}
			return Number(math.Mod(leftValue, rightValue)), nil
		}
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		if left.Kind() != right.Kind() {
			return Null(), newError(source, binary.pos, fmt.Sprintf("比较运算需要同类型的值，得到%s和%s", left.Format(), right.Format()))
		}
		switch left.Kind() {
		case KindNumber:
			leftValue, _ := left.Number()
			rightValue, _ := right.Number()
			return Bool(compare(binary.operator, leftValue, rightValue)), nil
		case KindString:
			leftValue, _ := left.String()
			rightValue, _ := right.String()
			return Bool(compare(binary.operator, leftValue, rightValue)), nil
		default:
			return Null(), newError(source, binary.pos, "比较运算只支持数值和字符串")
		}
	case tokenEqual:
		return Bool(left.Equal(right)), nil
	case tokenNotEqual:
		return Bool(!left.Equal(right)), nil
	}
	return Null(), newError(source, binary.pos, "不支持的运算符")
}

type ordered interface {
	~float64 | ~string
}

func compare[T ordered](operator tokenKind, left, right T) bool {
	switch operator {
	case tokenLess:
		return left < right
	case tokenLessEqual:
		return left <= right
	case tokenGreater:
		return left > right
	case tokenGreaterEqual:
		return left >= right
	default:
		return false
	}
}

func evalCall(source string, call *callNode, env Env) (Value, error) {
	args := make([]Value, len(call.args))
	for i, arg := range call.args {
		value, err := evalNode(source, arg, env)
		if err != nil {
			return Null(), err
		}
		args[i] = value
	}
	switch call.name {
	case "min", "max":
		numbers, err := flattenNumbers(source, call.pos, call.name, args)
		if err != nil {
			return Null(), err
		}
		result := numbers[0]
		for _, number := range numbers[1:] {
			if call.name == "min" && number < result {
				result = number
			}
			if call.name == "max" && number > result {
				result = number
			}
		}
		return Number(result), nil
	case "abs":
		value, err := oneNumber(source, call, args[0])
		if err != nil {
			return Null(), err
		}
		return Number(math.Abs(value)), nil
	case "round":
		value, err := oneNumber(source, call, args[0])
		if err != nil {
			return Null(), err
		}
		return Number(math.Round(value)), nil
	case "floor":
		value, err := oneNumber(source, call, args[0])
		if err != nil {
			return Null(), err
		}
		return Number(math.Floor(value)), nil
	case "ceil":
		value, err := oneNumber(source, call, args[0])
		if err != nil {
			return Null(), err
		}
		return Number(math.Ceil(value)), nil
	case "if":
		condition, ok := args[0].Bool()
		if !ok {
			return Null(), newError(source, call.pos, fmt.Sprintf("函数 if 的条件必须是布尔值，得到%s", args[0].Format()))
		}
		if condition {
			return args[1], nil
		}
		return args[2], nil
	case "coalesce":
		for _, arg := range args {
			if !arg.IsNull() {
				return arg, nil
			}
		}
		return Null(), nil
	case "sum", "avg":
		numbers, err := flattenNumbers(source, call.pos, call.name, args)
		if err != nil {
			return Null(), err
		}
		total := 0.0
		for _, number := range numbers {
			total += number
		}
		if call.name == "avg" {
			total /= float64(len(numbers))
		}
		return Number(total), nil
	case "count":
		count := 0
		for _, arg := range args {
			if items, ok := arg.List(); ok {
				count += len(items)
				continue
			}
			if !arg.IsNull() {
				count++
			}
		}
		return Number(float64(count)), nil
	}
	return Null(), newError(source, call.pos, fmt.Sprintf("不允许调用函数 %q", call.name))
}

func oneNumber(source string, call *callNode, value Value) (float64, error) {
	number, ok := value.numeric()
	if !ok {
		return 0, newError(source, call.pos, fmt.Sprintf("函数 %s 需要数值参数，得到%s", call.name, value.Format()))
	}
	return number, nil
}

func flattenNumbers(source string, pos int, name string, args []Value) ([]float64, error) {
	var numbers []float64
	for _, arg := range args {
		if items, ok := arg.List(); ok {
			for _, item := range items {
				number, ok := item.numeric()
				if !ok {
					return nil, newError(source, pos, fmt.Sprintf("函数 %s 需要数值参数，得到%s", name, item.Format()))
				}
				numbers = append(numbers, number)
			}
			continue
		}
		number, ok := arg.numeric()
		if !ok {
			return nil, newError(source, pos, fmt.Sprintf("函数 %s 需要数值参数，得到%s", name, arg.Format()))
		}
		numbers = append(numbers, number)
	}
	if len(numbers) == 0 {
		return nil, newError(source, pos, fmt.Sprintf("函数 %s 没有可计算的数值", name))
	}
	return numbers, nil
}
