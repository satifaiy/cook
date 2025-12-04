package ast

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/cozees/cook/pkg/cook/token"
	"github.com/cozees/cook/pkg/runtime/args"
)

func indexes(ctx Context, ns ...Node) (rg []int, err error) {
	for _, n := range ns {
		si, sk, err := n.Evaluate(ctx)
		if err != nil {
			return nil, err
		} else if sk != reflect.Int64 {
			return nil, fmt.Errorf("expression %s is an integer value", n)
		}
		rg = append(rg, int(si.(int64)))
	}
	sort.Ints(rg)
	return
}

func stringOf(ctx Context, ns ...Node) (ses []string, err error) {
	for _, n := range ns {
		si, sk, err := n.Evaluate(ctx)
		if err != nil {
			return nil, err
		} else if sk != reflect.String {
			return nil, fmt.Errorf("expression %s is not a valid string", n)
		}
		ses = append(ses, si.(string))
	}
	return
}

func convertToNum(s string) (any, reflect.Kind, error) {
	if iv, err := strconv.ParseInt(s, 10, 64); err == nil {
		return iv, reflect.Int64, nil
	} else if fv, err := strconv.ParseFloat(s, 64); err == nil {
		return fv, reflect.Float64, nil
	} else {
		return nil, reflect.Invalid, err
	}
}

func convertToFloat(ctx Context, val any, kind reflect.Kind) (float64, error) {
	switch kind {
	case reflect.Int64:
		return float64(val.(int64)), nil
	case reflect.Float64:
		return val.(float64), nil
	case reflect.String:
		return strconv.ParseFloat(val.(string), 64)
	default:
		return 0, fmt.Errorf("value %v cannot cast/convert to float", val)
	}
}

func convertToInt(ctx Context, val any, kind reflect.Kind) (int64, error) {
	switch kind {
	case reflect.Int64:
		return val.(int64), nil
	case reflect.Float64:
		return 0, fmt.Errorf("value %v will be cut when cast to integer, use integer(number) instead", val)
	case reflect.String:
		return strconv.ParseInt(val.(string), 10, 64)
	default:
		return 0, fmt.Errorf("value %v cannot cast to integer", val)
	}
}

func convertToString(ctx Context, val any, kind reflect.Kind) (string, error) {
	switch kind {
	case reflect.Int64:
		return strconv.FormatInt(val.(int64), 10), nil
	case reflect.Float64:
		return strconv.FormatFloat(val.(float64), 'g', -1, 64), nil
	case reflect.Bool:
		return strconv.FormatBool(val.(bool)), nil
	case reflect.String:
		return val.(string), nil
	default:
		if b, ok := val.([]byte); ok {
			return string(b), nil
		} else {
			return "", fmt.Errorf("value %v cannot cast to string", val)
		}
	}
}

func expandArrayTo(ctx Context, rv reflect.Value, array []string) ([]string, error) {
	var err error
	var sv string
	for i := 0; i < rv.Len(); i++ {
		v := rv.Index(i)
		switch v.Kind() {
		case reflect.Array, reflect.Slice:
			if array, err = expandArrayTo(ctx, v, array); err != nil {
				return nil, err
			}
		default:
			i := v.Interface()
			if sv, err = convertToString(ctx, i, reflect.ValueOf(i).Kind()); err != nil {
				return nil, err
			} else {
				array = append(array, sv)
			}
		}
	}
	return array, nil
}

func expandArrayToFuncArgs(ctx Context, rv reflect.Value, array []*args.FunctionArg) []*args.FunctionArg {
	for i := 0; i < rv.Len(); i++ {
		v := rv.Index(i)
		switch v.Kind() {
		case reflect.Array, reflect.Slice:
			array = expandArrayToFuncArgs(ctx, v, array)
		default:
			i := v.Interface()
			array = append(array, &args.FunctionArg{Val: i, Kind: reflect.ValueOf(i).Kind()})
		}
	}
	return array
}

func addOperator(ctx Context, vl, vr any, vkl, vkr reflect.Kind) (any, reflect.Kind, error) {
	// array operation
	// 0: 1 + ["a", 2, 3.5] => [1, "a", 2, 3.5]
	// 1: ["b", 123, true] + 2.1 => ["b", 123, true, 2.1]
	// 2: [1, 2] + ["a", true] => [1, 2, "a", true]
	head := 0
	switch {
	case vkl == reflect.Slice:
		if vkr == reflect.Slice {
			head = 2
		} else {
			head = 1
		}
		goto opOnArray
	case vkr == reflect.Slice:
		goto opOnArray
	case vkl == reflect.String || vkr == reflect.String:
		if s1, err := convertToString(ctx, vl, vkl); err != nil {
			return nil, 0, err
		} else if s2, err := convertToString(ctx, vr, vkr); err != nil {
			return nil, 0, err
		} else {
			return s1 + s2, reflect.String, nil
		}
	case vkl == reflect.Float64 || vkr == reflect.Float64:
		if f1, err := convertToFloat(ctx, vl, vkl); err != nil {
			return nil, 0, err
		} else if f2, err := convertToFloat(ctx, vr, vkr); err != nil {
			return nil, 0, err
		} else {
			return f1 + f2, reflect.Float64, nil
		}
	case vkl == reflect.Int64 && vkr == reflect.Int64:
		return vl.(int64) + vr.(int64), reflect.Int64, nil
	}
	// value is not suitable to operate with + operator
	return nil, reflect.Invalid, fmt.Errorf("operator + is not supported for value %v and %v", vl, vr)

opOnArray:
	switch head {
	case 0:
		return append([]any{vl}, vr.([]any)...), reflect.Slice, nil
	case 1:
		return append(vl.([]any), vr), reflect.Slice, nil
	case 2:
		return append(vl.([]any), vr.([]any)...), reflect.Slice, nil
	default:
		panic("illegal state for array operation")
	}
}

func numOperator(ctx Context, op token.Token, vl, vr any, vkl, vkr reflect.Kind) (any, reflect.Kind, error) {
	switch {
	case vkl == reflect.Float64 || vkr == reflect.Float64:
		if token.ADD < op && op < token.REM {
			goto numFloat
		}
	case vkl == reflect.Int64 || vkr == reflect.Int64:
		if token.ADD < op && op < token.LAND {
			goto numInt
		}
	}
	// value is not suitable
	return nil, reflect.Invalid, fmt.Errorf("operator %s is not supported for value %v and %v", op, vl, vr)

numFloat:
	if fa, err := convertToFloat(ctx, vl, vkl); err != nil {
		return nil, 0, err
	} else if fb, err := convertToFloat(ctx, vr, vkr); err != nil {
		return nil, 0, err
	} else {
		switch op {
		case token.SUB:
			return fa - fb, reflect.Float64, nil
		case token.MUL:
			return fa * fb, reflect.Float64, nil
		case token.QUO:
			return fa / fb, reflect.Float64, nil
		default:
			panic("illegal state operation")
		}
	}
numInt:
	if ia, err := convertToInt(ctx, vl, vkl); err != nil {
		return nil, 0, err
	} else if ib, err := convertToInt(ctx, vr, vkr); err != nil {
		return nil, 0, err
	} else {
		switch op {
		case token.SUB:
			return ia - ib, reflect.Int64, nil
		case token.MUL:
			return ia * ib, reflect.Int64, nil
		case token.QUO:
			return ia / ib, reflect.Int64, nil
		case token.REM:
			return ia % ib, reflect.Int64, nil
		case token.AND:
			return ia & ib, reflect.Int64, nil
		case token.OR:
			return ia | ib, reflect.Int64, nil
		case token.XOR:
			return ia ^ ib, reflect.Int64, nil
		case token.SHL:
			return ia << ib, reflect.Int64, nil
		case token.SHR:
			return ia >> ib, reflect.Int64, nil
		default:
			panic("illegal state operation")
		}
	}
}

func logicOperator(ctx Context, op token.Token, vl, vr any, vkl, vkr reflect.Kind) (any, reflect.Kind, error) {
	if (vkl == reflect.Float64 || vkl == reflect.Int64) &&
		(vkr == reflect.Float64 || vkr == reflect.Int64) {
		if fl, err := convertToFloat(ctx, vl, vkl); err != nil {
			return nil, 0, err
		} else if fr, err := convertToFloat(ctx, vr, vkr); err != nil {
			return nil, 0, err
		} else {
			switch op {
			case token.EQL:
				return fl == fr, reflect.Bool, nil
			case token.LSS:
				return fl < fr, reflect.Bool, nil
			case token.GTR:
				return fl > fr, reflect.Bool, nil
			case token.NEQ:
				return fl != fr, reflect.Bool, nil
			case token.LEQ:
				return fl <= fr, reflect.Bool, nil
			case token.GEQ:
				return fl >= fr, reflect.Bool, nil
			default:
				panic("illegal state operator")
			}
		}
	} else if vkl == vkr {
		// any type other than integer or float must has the same type
		switch vkl {
		case reflect.Bool:
			if op == token.EQL {
				return vl.(bool) == vr.(bool), reflect.Bool, nil
			} else if op == token.NEQ {
				return vl.(bool) != vr.(bool), reflect.Bool, nil
			}
		case reflect.String:
			r := strings.Compare(vl.(string), vr.(string))
			switch op {
			case token.EQL:
				return r == 0, reflect.Bool, nil
			case token.LSS:
				return r < 0, reflect.Bool, nil
			case token.GTR:
				return r > 0, reflect.Bool, nil
			case token.NEQ:
				return r != 0, reflect.Bool, nil
			case token.LEQ:
				return r <= 0, reflect.Bool, nil
			case token.GEQ:
				return r >= 0, reflect.Bool, nil
			}
		default:
			if op == token.EQL {
				return vl == vr, reflect.Bool, nil
			} else if op == token.NEQ {
				return vl != vr, reflect.Bool, nil
			}
		}
	}
	// value is not suitable
	return nil, reflect.Invalid, fmt.Errorf("operator %s is not supported for value %v and %v", op, vl, vr)
}
