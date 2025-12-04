package ast

import (
	"reflect"
	"testing"

	"github.com/cozees/cook/pkg/cook/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func expectVar(t *testing.T, ctx Context, name string, v any, k reflect.Kind) {
	rv, rk, _ := ctx.GetVariable(name)
	assert.Equal(t, k, rk)
	assert.Equal(t, v, rv)
}

func TestStatement(t *testing.T) {
	ctx := NewCook().(*cook).renewContext()
	bs := &BlockStatement{
		Stmts: []Statement{
			&AssignStatement{
				Ident: &Ident{Name: "var1"},
				Op:    token.ASSIGN,
				Value: &Binary{
					L:  &BasicLit{Lit: "28", Kind: token.INTEGER},
					Op: token.ADD,
					R:  &BasicLit{Lit: "9.3", Kind: token.FLOAT},
				},
			},
			&AssignStatement{
				Ident: &Ident{Name: "var2"},
				Op:    token.ASSIGN,
				Value: &Binary{
					L:  &Ident{Name: "var1"},
					Op: token.MUL,
					R:  &BasicLit{Lit: "100", Kind: token.INTEGER},
				},
			},
			&AssignStatement{
				Ident: &Ident{Name: "var3"},
				Op:    token.ASSIGN,
				Value: &ArrayLiteral{
					Values: []Node{
						&BasicLit{Lit: "34", Kind: token.INTEGER},
						&BasicLit{Lit: "true", Kind: token.BOOLEAN},
						&BasicLit{Lit: "text", Kind: token.STRING},
						&BasicLit{Lit: "9.2", Kind: token.FLOAT},
					},
				},
			},
			&AssignStatement{
				Ident: &Ident{Name: "var1"},
				Op:    token.MUL_ASSIGN,
				Value: &Index{
					X:     &Ident{Name: "var3"},
					Index: &BasicLit{Lit: "0", Kind: token.INTEGER},
				},
			},
		},
	}
	require.NoError(t, bs.Evaluate(ctx))
	// check variable
	expectVar(t, ctx, "var1", (float64(28)+9.3)*float64(34), reflect.Float64)
	expectVar(t, ctx, "var2", (float64(28)+9.3)*float64(100), reflect.Float64)
	expectVar(t, ctx, "var3", []any{int64(34), true, "text", 9.2}, reflect.Slice)
}

func TestIfElse(t *testing.T) {
	stmt := &IfStatement{
		Cond: &Binary{L: &Ident{Name: "a"}, Op: token.EQL, R: &Ident{Name: "b"}},
		Insts: &BlockStatement{
			Stmts: []Statement{
				&AssignStatement{
					Ident: &Ident{Name: "a"},
					Op:    token.ASSIGN,
					Value: &BasicLit{Lit: "23", Kind: token.INTEGER},
				},
			},
		},
		Else: &ElseStatement{
			IfStmt: &IfStatement{
				Cond: &Binary{L: &Ident{Name: "a"}, Op: token.GTR, R: &BasicLit{Lit: "2", Kind: token.INTEGER}},
				Insts: &BlockStatement{
					Stmts: []Statement{
						&AssignStatement{
							Ident: &Ident{Name: "b"},
							Op:    token.ASSIGN,
							Value: &BasicLit{Lit: "314", Kind: token.STRING},
						},
					},
				},
			},
		},
	}
	ctx := NewCook().(*cook).renewContext()
	ctx.SetVariable("a", float64(12), reflect.Float64, nil)
	ctx.SetVariable("b", float64(12), reflect.Float64, nil)
	require.NoError(t, stmt.Evaluate(ctx))
	expectVar(t, ctx, "a", int64(23), reflect.Int64)
	expectVar(t, ctx, "b", 12.0, reflect.Float64)
	ctx.SetVariable("b", float64(11), reflect.Float64, nil)
	require.NoError(t, stmt.Evaluate(ctx))
	expectVar(t, ctx, "a", int64(23), reflect.Int64)
	expectVar(t, ctx, "b", "314", reflect.String)
}

func TestForStatement(t *testing.T) {
	stf := &ForStatement{
		I: &Ident{Name: "i"},
		Range: &Interval{
			A: &BasicLit{Lit: "1", Kind: token.INTEGER},
			B: &BasicLit{Lit: "4", Kind: token.INTEGER},
		},
		Insts: &BlockStatement{
			Stmts: []Statement{&AssignStatement{Ident: &Ident{Name: "a"}, Op: token.ADD_ASSIGN, Value: &Ident{Name: "i"}}},
		},
	}
	ctx = NewCook().(*cook).renewContext()
	ctx.SetVariable("a", make([]any, 0), reflect.Slice, nil)
	err := stf.Evaluate(ctx)
	require.NoError(t, err)
	expectVar(t, ctx, "a", []any{int64(2), int64(3)}, reflect.Slice)
	// test [1..4]
	stf.Range.AInclude = true
	stf.Range.BInclude = true
	ctx.SetVariable("a", make([]any, 0), reflect.Slice, nil)
	err = stf.Evaluate(ctx)
	require.NoError(t, err)
	expectVar(t, ctx, "a", []any{int64(1), int64(2), int64(3), int64(4)}, reflect.Slice)
	// test array
	stf.Range = nil
	stf.Value = &Ident{Name: "v"}
	stf.Oprnd = &ArrayLiteral{Values: indexesNode(4, 5, 6)}
	stf.Insts.Stmts = append(stf.Insts.Stmts, &AssignStatement{
		Ident: &Ident{Name: "a"}, Op: token.ADD_ASSIGN, Value: &Ident{Name: "v"},
	})
	ctx.SetVariable("a", make([]any, 0), reflect.Slice, nil)
	err = stf.Evaluate(ctx)
	require.NoError(t, err)
	expectVar(t, ctx, "a", []any{int64(0), int64(4), int64(1), int64(5), int64(2), int64(6)}, reflect.Slice)
	// test map
	stf.Oprnd = &MapLiteral{Keys: indexesNode(8, 7, 3), Values: indexesNode(4, 5, 6)}
	stf.Insts.Stmts = []Statement{
		&AssignStatement{
			Ident: &Ident{Name: "a"},
			Op:    token.ADD_ASSIGN,
			Value: &MergeMap{
				Op: token.LSS,
				Value: &MapLiteral{
					Keys:   []Node{&Ident{Name: "i"}},
					Values: []Node{&Ident{Name: "v"}},
				},
			},
		},
	}
	ctx.SetVariable("a", make(map[any]any), reflect.Map, nil)
	err = stf.Evaluate(ctx)
	require.NoError(t, err)
	expectVar(t, ctx, "a", map[any]any{int64(8): int64(4), int64(7): int64(5), int64(3): int64(6)}, reflect.Map)
}

func TestBreakContinue(t *testing.T) {
	vari := &Ident{Name: "i"}
	vara, varb, varc := &Ident{Name: "a"}, &Ident{Name: "b"}, &Ident{Name: "c"}
	lit0 := &BasicLit{Lit: "0", Kind: token.INTEGER}
	lit2 := &BasicLit{Lit: "2", Kind: token.INTEGER}
	lit10 := &BasicLit{Lit: "10", Kind: token.INTEGER}
	stf := &ForStatement{
		I: vari,
		Range: &Interval{
			A: &BasicLit{Lit: "0", Kind: token.INTEGER},
			B: &BasicLit{Lit: "21", Kind: token.INTEGER},
		},
		Insts: &BlockStatement{
			Stmts: []Statement{
				&IfStatement{
					Cond: &Binary{L: &Binary{L: vari, Op: token.REM, R: lit2}, Op: token.EQL, R: lit0},
					Insts: &BlockStatement{
						Stmts: []Statement{
							&AssignStatement{Ident: varb, Op: token.ADD_ASSIGN, Value: vari},
							&BreakContinueStatement{Op: token.CONTINUE},
						},
					},
					Else: &ElseStatement{
						IfStmt: &IfStatement{
							Cond: &Binary{L: varc, Op: token.LSS, R: lit10},
							Insts: &BlockStatement{
								Stmts: []Statement{
									&BreakContinueStatement{Op: token.BREAK},
								},
							},
						},
					},
				},
				&AssignStatement{Ident: vara, Op: token.MUL_ASSIGN, Value: vari},
			},
		},
	}
	ctx := NewCook().(*cook).renewContext()
	ctx.SetVariable(vara.Name, int64(1), reflect.Int64, nil)
	ctx.SetVariable(varb.Name, int64(1), reflect.Int64, nil)
	ctx.SetVariable(varc.Name, int64(15), reflect.Int64, nil)
	// test continue
	err := stf.Evaluate(ctx)
	require.NoError(t, err)
	expecta, expectb := 1, 1
	for i := 1; i < 21; i++ {
		if i%2 == 0 {
			expectb += i
			continue
		}
		expecta *= i
	}
	expectVar(t, ctx, vara.Name, int64(expecta), reflect.Int64)
	expectVar(t, ctx, varb.Name, int64(expectb), reflect.Int64)
	expectVar(t, ctx, varc.Name, int64(15), reflect.Int64)
	// test break
	ctx.SetVariable(vara.Name, int64(1), reflect.Int64, nil)
	ctx.SetVariable(varb.Name, int64(1), reflect.Int64, nil)
	ctx.SetVariable(varc.Name, int64(2), reflect.Int64, nil)
	err = stf.Evaluate(ctx)
	require.NoError(t, err)
	expectVar(t, ctx, vara.Name, int64(1), reflect.Int64)
	expectVar(t, ctx, varb.Name, int64(1), reflect.Int64)
	expectVar(t, ctx, varc.Name, int64(2), reflect.Int64)
}

func TestNestingLoop(t *testing.T) {
	vari1, vari2, vari3 := &Ident{Name: "i1"}, &Ident{Name: "i2"}, &Ident{Name: "i3"}
	varxa, varxb, varxc := &Ident{Name: "a"}, &Ident{Name: "b"}, &Ident{Name: "c"}
	ilit0 := &BasicLit{Lit: "0", Kind: token.INTEGER}
	ilit2 := &BasicLit{Lit: "2", Kind: token.INTEGER}
	ilit3 := &BasicLit{Lit: "3", Kind: token.INTEGER}
	for3 := &ForStatement{
		I: vari3,
		Range: &Interval{
			A: &BasicLit{Lit: "0", Kind: token.INTEGER},
			B: &BasicLit{Lit: "11", Kind: token.INTEGER},
		},
		Insts: &BlockStatement{
			Stmts: []Statement{
				&IfStatement{
					Cond: &Binary{
						L: &Binary{
							L: &Binary{
								L:  vari3,
								Op: token.ADD,
								R: &Binary{
									L:  vari1,
									Op: token.MUL,
									R:  vari2,
								},
							}, Op: token.REM, R: ilit2,
						}, Op: token.EQL, R: ilit0,
					},
					Insts: &BlockStatement{
						Stmts: []Statement{
							&AssignStatement{Ident: varxb, Op: token.ADD_ASSIGN, Value: varxc},
							&ExprWrapperStatement{X: &IncDec{Op: token.INC, X: varxc}},
							&BreakContinueStatement{Label: "labelb", Op: token.CONTINUE},
						},
					},
					Else: &ElseStatement{IfStmt: &IfStatement{
						Cond: &Binary{L: &Binary{L: vari3, Op: token.REM, R: ilit3}, Op: token.EQL, R: ilit0},
						Insts: &BlockStatement{
							Stmts: []Statement{
								&AssignStatement{Ident: varxc, Op: token.ADD_ASSIGN, Value: &Binary{L: varxb, Op: token.ADD, R: varxa}},
								&BreakContinueStatement{Label: "labela", Op: token.CONTINUE},
							},
						},
						Else: &ElseStatement{IfStmt: &IfStatement{
							Cond: &Binary{
								L: &Binary{
									L: &Binary{
										L:  vari3,
										Op: token.ADD,
										R:  vari2,
									},
									Op: token.REM,
									R:  ilit2,
								},
								Op: token.EQL,
								R:  ilit0,
							},
							Insts: &BlockStatement{
								Stmts: []Statement{
									&AssignStatement{Ident: varxa, Op: token.ADD_ASSIGN, Value: ilit2},
									&BreakContinueStatement{Op: token.BREAK},
								},
							},
						}},
					}},
				},
				&AssignStatement{Ident: varxb, Op: token.ADD_ASSIGN, Value: varxa},
			},
		},
	}
	for2 := &ForStatement{
		Label: "labelb",
		I:     vari2,
		Range: &Interval{
			A: &BasicLit{Lit: "5", Kind: token.INTEGER},
			B: &BasicLit{Lit: "11", Kind: token.INTEGER},
		},
		Insts: &BlockStatement{
			Stmts: []Statement{
				for3,
				&AssignStatement{
					Ident: varxc,
					Op:    token.ADD_ASSIGN,
					Value: &Binary{L: varxb, Op: token.ADD, R: vari2},
				},
			},
		},
	}
	for1 := &ForStatement{
		Label: "labela",
		I:     vari1,
		Range: &Interval{
			A: &BasicLit{Lit: "14", Kind: token.INTEGER},
			B: &BasicLit{Lit: "21", Kind: token.INTEGER},
		},
		Insts: &BlockStatement{
			Stmts: []Statement{
				for2,
				&AssignStatement{
					Ident: varxa,
					Op:    token.ADD_ASSIGN,
					Value: &Binary{L: vari1, Op: token.ADD, R: varxb},
				},
			},
		},
	}
	ctx := NewCook().(*cook).renewContext()
	ctx.SetVariable("a", int64(1), reflect.Int64, nil)
	ctx.SetVariable("b", int64(1), reflect.Int64, nil)
	ctx.SetVariable("c", int64(1), reflect.Int64, nil)
	err := for1.Evaluate(ctx)
	require.NoError(t, err)
	//
	exa, exb, exc := 1, 1, 1
labela:
	for i1 := 15; i1 < 21; i1++ {
	labelb:
		for i2 := 6; i2 < 11; i2++ {
			for i3 := 1; i3 < 11; i3++ {
				if (i3+(i1*i2))%2 == 0 {
					exb += exc
					exc++
					continue labelb
				} else if i3%3 == 0 {
					exc += exb + exa
					continue labela
				} else if (i3+i2)%2 == 0 {
					exa += 2
					break
				}
				exb += exa
			}
			exc += exb + i2
		}
		exa += i1 + exb
	}
	expectVar(t, ctx, "a", int64(exa), reflect.Int64)
	expectVar(t, ctx, "b", int64(exb), reflect.Int64)
	expectVar(t, ctx, "c", int64(exc), reflect.Int64)
}
