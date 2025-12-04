package ast

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/cozees/cook/pkg/cook/token"
	"github.com/stretchr/testify/require"
)

func astRange(a, b int) *Interval {
	return &Interval{
		A: &BasicLit{Lit: strconv.Itoa(a), Kind: token.INTEGER},
		B: &BasicLit{Lit: strconv.Itoa(b), Kind: token.INTEGER},
	}
}

func TestCook(t *testing.T) {
	vara, varb, varc := &Ident{Name: "a"}, &Ident{Name: "b"}, &Ident{Name: "c"}
	cook := NewCook().(*cook)
	cook.Insts = &BlockStatement{
		Stmts: []Statement{
			&AssignStatement{Ident: vara, Op: token.ASSIGN, Value: &BasicLit{Lit: "342", Kind: token.INTEGER}},
			&AssignStatement{Ident: varb, Op: token.ASSIGN, Value: &BasicLit{Lit: "text", Kind: token.STRING}},
			&AssignStatement{Ident: varc, Op: token.ASSIGN, Value: &BasicLit{Lit: "442", Kind: token.FLOAT}},
		},
	}
	target1, err := cook.AddTarget(nil, "target1")
	require.NoError(t, err)
	target1.Insts.Stmts = []Statement{
		&ForStatement{
			I:     &Ident{Name: "i"},
			Range: astRange(0, 21),
			Insts: &BlockStatement{
				Stmts: []Statement{
					&AssignStatement{Ident: vara, Op: token.ADD_ASSIGN, Value: &Ident{Name: "i"}},
				},
			},
		},
	}
	target2, err := cook.AddTarget(nil, "target2")
	require.NoError(t, err)
	target2.Insts.Stmts = []Statement{
		&ForStatement{
			I:     &Ident{Name: "i"},
			Range: astRange(50, 15),
			Insts: &BlockStatement{
				Stmts: []Statement{
					&AssignStatement{Ident: varb, Op: token.ADD_ASSIGN, Value: &Ident{Name: "i"}},
				},
			},
		},
	}
	target3, err := cook.AddTarget(nil, "target3")
	require.NoError(t, err)
	target3.Insts.Stmts = []Statement{
		&ForStatement{
			I:     &Ident{Name: "i"},
			Range: astRange(200, 1005),
			Insts: &BlockStatement{
				Stmts: []Statement{
					&AssignStatement{Ident: varc, Op: token.ADD_ASSIGN, Value: &Ident{Name: "i"}},
				},
			},
		},
	}
	cook.initializeTargets = []*Target{
		{name: "initialize", Insts: &BlockStatement{Stmts: []Statement{
			&AssignStatement{Ident: vara, Op: token.MUL_ASSIGN, Value: &BasicLit{Lit: "2", Kind: token.INTEGER}},
		}}},
		{name: "initialize", Insts: &BlockStatement{Stmts: []Statement{
			&AssignStatement{Ident: varb, Op: token.ADD_ASSIGN, Value: &BasicLit{Lit: "27", Kind: token.INTEGER}},
		}}},
	}
	cook.finalizeTargets = []*Target{
		{name: "finalize", Insts: &BlockStatement{Stmts: []Statement{
			&AssignStatement{Ident: vara, Op: token.MUL_ASSIGN, Value: &BasicLit{Lit: "2", Kind: token.INTEGER}},
		}}},
		{name: "finalize", Insts: &BlockStatement{Stmts: []Statement{
			&AssignStatement{Ident: varc, Op: token.QUO_ASSIGN, Value: &BasicLit{Lit: "2", Kind: token.INTEGER}},
		}}},
	}
	cook.targetAll = &Target{name: "all", Insts: &BlockStatement{Stmts: []Statement{
		&ExprWrapperStatement{X: &Call{Kind: token.AT, Name: "target1"}},
		&ExprWrapperStatement{X: &Call{Kind: token.AT, Name: "target2"}},
		&ExprWrapperStatement{X: &Call{Kind: token.AT, Name: "target3"}},
	}}}
	// execute all && exectue explicit name
	for i := range 2 {
		if i == 0 {
			require.NoError(t, cook.Execute(nil))
		} else {
			require.NoError(t, cook.ExecuteWithTarget(nil, "target1", "target2", "target3"))
		}
		exa, exb, exc := 342*2, "text27", 442.0
		for i := 1; i < 21; i++ {
			exa += i
		}
		for i := 49; i > 15; i-- {
			exb += strconv.Itoa(i)
		}
		for i := 201; i < 1005; i++ {
			exc += float64(i)
		}
		exa *= 2
		exc /= 2
		expectVar(t, cook.ctx, vara.Name, int64(exa), reflect.Int64)
		expectVar(t, cook.ctx, varb.Name, exb, reflect.String)
		expectVar(t, cook.ctx, varc.Name, exc, reflect.Float64)
	}
}
