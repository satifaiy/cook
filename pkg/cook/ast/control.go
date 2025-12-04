package ast

import (
	"fmt"
	"reflect"
)

type ForStatement struct {
	*Base
	Label string
	I     *Ident
	Value *Ident
	Oprnd Node      // must be map, array or string
	Range *Interval // {1..2}, (0..3), [1..2]:1
	Insts *BlockStatement
}

func (fst *ForStatement) Evaluate(ctx Context) error {
	var nb bool
	var ni, val any
	if fst.Oprnd != nil {
		v, vk, err := fst.Oprnd.Evaluate(ctx)
		if err == nil {
			switch vk {
			case reflect.Slice:
				vv := reflect.ValueOf(v)
				scope, lid := ctx.EnterBlock(true, fst.Label)
				defer ctx.ExitBlock(lid)
				for i := 0; i < vv.Len(); i++ {
					scope.SetVariable(fst.I.Name, int64(i), reflect.Int64, nil)
					iv := vv.Index(i)
					kind := iv.Kind()
					if kind == reflect.Interface {
						kind = iv.Elem().Kind()
					}
					scope.SetVariable(fst.Value.Name, iv.Interface(), kind, nil)
					nb, _, val, err = fst.executeInsts(ctx, reflect.Invalid, kind)
					if val != nil {
						iv.Set(reflect.ValueOf(val))
					}
					if err != nil || nb {
						break
					}
				}
			case reflect.Map:
				vv := reflect.ValueOf(v)
				scope, lid := ctx.EnterBlock(true, fst.Label)
				defer ctx.ExitBlock(lid)
				for _, k := range vv.MapKeys() {
					kind := k.Kind()
					if kind == reflect.Interface {
						kind = k.Elem().Kind()
					}
					scope.SetVariable(fst.I.Name, k.Interface(), kind, nil)
					iv := vv.MapIndex(k)
					kind = iv.Kind()
					if kind == reflect.Interface {
						kind = iv.Elem().Kind()
					}
					scope.SetVariable(fst.Value.Name, iv.Interface(), kind, nil)
					nb, _, val, err = fst.executeInsts(ctx, reflect.Invalid, kind)
					if val != nil {
						vv.SetMapIndex(k, reflect.ValueOf(val))
					}
					if err != nil || nb {
						break
					}
				}
			case reflect.String:
				scope, lid := ctx.EnterBlock(true, fst.Label)
				defer ctx.ExitBlock(lid)
				for i, r := range v.(string) {
					scope.SetVariable(fst.I.Name, int64(i), reflect.Int64, nil)
					scope.SetVariable(fst.Value.Name, string(r), reflect.String, nil)
					if nb, _, _, err = fst.executeInsts(ctx, reflect.Invalid, reflect.Invalid); err != nil || nb {
						break
					}
				}
			default:
				return fmt.Errorf("for loop operand %s is not allowed, only map, array or string is allowed", fst.Oprnd)
			}
		}
		return err
	} else if fst.Range == nil {
		// infinite loop, only break can break out of loop
		var err error
		_, lid := ctx.EnterBlock(true, fst.Label)
		defer ctx.ExitBlock(lid)
		for {
			if nb, _, _, err = fst.executeInsts(ctx, reflect.Invalid, reflect.Invalid); err != nil || nb {
				break
			}
		}
		return err
	} else if rg, _, err := fst.Range.Evaluate(ctx); err != nil {
		return err
	} else {
		if flrg, ok := rg.([]float64); ok && len(flrg) == 3 {
			// we know step must be either integer or float
			step := flrg[2]
			scope, lid := ctx.EnterBlock(true, fst.Label)
			defer ctx.ExitBlock(lid)
			if flrg[0] > flrg[1] {
				for i := flrg[0]; i >= flrg[1]; i -= step {
					scope.SetVariable(fst.I.Name, i, reflect.Float64, nil)
					if nb, ni, _, err = fst.executeInsts(ctx, reflect.Float64, reflect.Invalid); err != nil || nb {
						break
					}
					i = ni.(float64)
				}
			} else {
				for i := flrg[0]; i <= flrg[1]; i += step {
					scope.SetVariable(fst.I.Name, i, reflect.Float64, nil)
					if nb, ni, _, err = fst.executeInsts(ctx, reflect.Float64, reflect.Invalid); err != nil || nb {
						break
					}
					i = ni.(float64)
				}
			}
		} else if ilrg := rg.([]int64); len(ilrg) == 3 {
			// let cook crash if rg is not integer. Range must evaluation must handle type check
			// before getting here. If it escape, it need to fixed. Test case already in place
			// thus this cast should be safe

			// we know step must be either integer or float
			step := ilrg[2]
			scope, lid := ctx.EnterBlock(true, fst.Label)
			defer ctx.ExitBlock(lid)
			if ilrg[0] > ilrg[1] {
				// (1..1)
				for i := ilrg[0]; i >= ilrg[1]; i -= step {
					scope.SetVariable(fst.I.Name, i, reflect.Int64, nil)
					if nb, ni, _, err = fst.executeInsts(ctx, reflect.Int64, reflect.Invalid); err != nil || nb {
						break
					}
					i = ni.(int64)
				}
			} else {
				for i := ilrg[0]; i <= ilrg[1]; i += step {
					scope.SetVariable(fst.I.Name, i, reflect.Int64, nil)
					if nb, ni, _, err = fst.executeInsts(ctx, reflect.Int64, reflect.Invalid); err != nil || nb {
						break
					}
					i = ni.(int64)
				}
			}
		}
		return err
	}
}

func (fst *ForStatement) executeInsts(ctx Context, ikind reflect.Kind, vkind reflect.Kind) (needBreak bool, index, val interface{}, err error) {
	if err = fst.Insts.Evaluate(ctx); err != nil {
		return false, 0, nil, err
	}
	sb := ctx.ShouldBreak(true)
	if !sb {
		ctx.ResetBreakContinue()
	}
	if ikind != reflect.Invalid {
		v, k, _ := ctx.GetVariable(fst.I.Name)
		if ikind != k {
			return false, 0, nil, fmt.Errorf("%s: for loop index type %s cannot be modfied to type %s", fst.I.ErrPos(), ikind, k)
		}
		return sb, v, nil, nil
	}
	if vkind != reflect.Invalid {
		v, k, _ := ctx.GetVariable(fst.Value.Name)
		if vkind != k {
			return false, 0, nil, fmt.Errorf("%s: for loop value type %s cannot be modfied to type %s", fst.I.ErrPos(), vkind, k)
		}
		return sb, -1, v, nil
	}
	return sb, -1, nil, nil
}

type IfStatement struct {
	*Base
	Cond  Node
	Insts *BlockStatement
	Else  *ElseStatement
}

func (ifst *IfStatement) Evaluate(ctx Context) error {
	if v, vk, err := ifst.Cond.Evaluate(ctx); err != nil {
		return err
	} else if vk != reflect.Bool {
		return fmt.Errorf("")
	} else if v.(bool) {
		return ifst.Insts.Evaluate(ctx)
	} else if ifst.Else != nil {
		return ifst.Else.Evaluate(ctx)
	}
	return nil
}

type ElseStatement struct {
	*Base
	IfStmt *IfStatement
	Insts  *BlockStatement
}

func (efst *ElseStatement) Evaluate(ctx Context) error {
	if efst.IfStmt != nil {
		return efst.IfStmt.Evaluate(ctx)
	} else {
		return efst.Insts.Evaluate(ctx)
	}
}
