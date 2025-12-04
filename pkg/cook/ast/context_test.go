package ast

import (
	"reflect"
	"testing"

	"github.com/cozees/cook/pkg/cook/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariableReference(t *testing.T) {
	ctx := NewCook().(*cook).renewContext()
	ctx.SetVariable("a", []any{
		[]any{int64(1), int64(2)},
		[]any{2.3, 9.2},
	}, reflect.Slice, nil)
	// test index manipulation
	ix := &Index{Index: &BasicLit{Lit: "1", Kind: token.INTEGER}, X: &Ident{Name: "a"}}
	as1 := &AssignStatement{Ident: &Ident{Name: "b"}, Op: token.ASSIGN, Value: ix}
	require.NoError(t, as1.Evaluate(ctx))
	as2 := &AssignStatement{Ident: &Ident{Name: "b"}, Op: token.ADD_ASSIGN, Value: &BasicLit{Lit: "text", Kind: token.STRING}}
	require.NoError(t, as2.Evaluate(ctx))
	v, k, _ := ctx.GetVariable("b")
	assert.Equal(t, reflect.Slice, k)
	assert.Equal(t, []any{2.3, 9.2, "text"}, v)
	v, k, _ = ctx.GetVariable("a")
	assert.Equal(t, reflect.Slice, k)
	assert.Equal(t, []any{
		[]any{int64(1), int64(2)},
		[]any{2.3, 9.2, "text"},
	}, v)
	a := make(map[any]any)
	a["a"] = []any{
		[]any{int64(1), int64(2)},
		[]any{2.3, 9.2},
	}
}
