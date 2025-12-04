package ast

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"

	"github.com/cozees/cook/pkg/cook/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = NewCook().(*cook).renewContext()
var dummyBase = &Base{
	Offset: 0,
	File:   token.NewFile("", 100),
}

var il1, il2 = &BasicLit{Lit: "12", Kind: token.INTEGER}, &BasicLit{Lit: "21", Kind: token.INTEGER}
var fl1, fl2 = &BasicLit{Lit: "5.2", Kind: token.FLOAT}, &BasicLit{Lit: "29.1", Kind: token.FLOAT}
var bl1, bl2 = &BasicLit{Lit: "true", Kind: token.BOOLEAN}, &BasicLit{Lit: "false", Kind: token.BOOLEAN}
var sl1, sl2 = &BasicLit{Lit: "98", Kind: token.STRING}, &BasicLit{Lit: "sample", Kind: token.STRING}
var keys []Node
var lmaps = make(map[any]any)
var basicSize int64
var echo = &BasicLit{Lit: "-e", Kind: token.STRING}
var curOSTok, nonOSTok = getOSToken()

func getOSToken() (a, b token.Token) {
	switch runtime.GOOS {
	case "linux":
		a = token.LINUX
		b = token.WINDOWS
	case "darwin":
		a = token.MACOS
		b = token.WINDOWS
	case "windows":
		a = token.WINDOWS
		b = token.LINUX
	}
	return
}

func init() {
	for i := 1; i < 9; i++ {
		keys = append(keys, &BasicLit{Lit: strconv.Itoa(i), Kind: token.INTEGER})
		lmaps[int64(i)] = (int64(i) + 1) * 2
	}
	ctx.SetVariable("var1", int64(123), reflect.Int64, nil)
	ctx.SetVariable("var2", 3.43, reflect.Float64, nil)
	ctx.SetVariable("var3", true, reflect.Bool, nil)
	ctx.SetVariable("var4", "sample text", reflect.String, nil)
	ctx.SetVariable("var5", []any{3.2, 2.2}, reflect.Slice, nil)
	ctx.SetVariable("var6", map[any]any{1.1: "xyz", "abc": int64(873)}, reflect.String, nil)
	ctx.SetVariable("var7", []any{int64(12), int64(21), 5.2, 29.1, true, false, "98", "sample"}, reflect.Slice, nil)
	ctx.SetVariable("var8", lmaps, reflect.Map, nil)
	stat, err := os.Stat("basic.go")
	if err != nil {
		panic(err)
	}
	basicSize = stat.Size()
}

type WrapHelper struct {
	*Base
	nodes []Node
}

func (wh *WrapHelper) Evaluate(ctx Context) (i any, k reflect.Kind, err error) {
	for _, n := range wh.nodes {
		if i, k, err = n.Evaluate(ctx); err != nil {
			break
		}
	}
	return
}

func (wh *WrapHelper) String() string       { return "" }
func (wh *WrapHelper) Visit(cb CodeBuilder) {}
func wrapExpr(nodes ...Node) Node           { return &WrapHelper{nodes: nodes} }

func indexesNode(inds ...int) (nodes []Node) {
	for _, ind := range inds {
		nodes = append(nodes, &BasicLit{Lit: strconv.Itoa(ind), Kind: token.INTEGER})
	}
	return
}

type ExprAstTestCase struct {
	node  Node
	value any
	kind  reflect.Kind
	isErr bool
}

var exprCases = []*ExprAstTestCase{
	{ // case 1
		node:  &BasicLit{Lit: "12", Kind: token.INTEGER},
		value: int64(12),
		kind:  reflect.Int64,
	},
	{ // case 2
		node:  &BasicLit{Lit: "true", Kind: token.BOOLEAN},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 3
		node:  &BasicLit{Lit: "12.3", Kind: token.FLOAT},
		value: 12.3,
		kind:  reflect.Float64,
	},
	{ // case 4
		node:  &BasicLit{Lit: "12.3", Kind: token.STRING},
		value: "12.3",
		kind:  reflect.String,
	},
	{ // case 5
		node: &Ident{Name: "var"},
	},
	{ // case 6
		node:  &Ident{Name: "var2"},
		value: 3.43,
		kind:  reflect.Float64,
	},
	{ // case 7
		node: &Conditional{
			Cond:  &BasicLit{Lit: "true", Kind: token.BOOLEAN},
			True:  &BasicLit{Lit: "123", Kind: token.INTEGER},
			False: &BasicLit{Lit: "5.3", Kind: token.FLOAT},
		},
		value: int64(123),
		kind:  reflect.Int64,
	},
	{ // case 8
		node: &Conditional{
			Cond:  &BasicLit{Base: dummyBase, Lit: "true", Kind: token.STRING},
			True:  &BasicLit{Lit: "123", Kind: token.INTEGER},
			False: &BasicLit{Lit: "5.3", Kind: token.FLOAT},
		},
		isErr: true,
	},
	{ // case 9
		node: &Fallback{
			Primary: &Ident{Name: "var1"},
			Default: &BasicLit{Lit: "0", Kind: token.INTEGER},
		},
		value: int64(123),
		kind:  reflect.Int64,
	},
	{ // case 10
		node: &Fallback{
			Primary: &Ident{Name: "var0"},
			Default: &BasicLit{Lit: "121", Kind: token.FLOAT},
		},
		value: 121.0,
		kind:  reflect.Float64,
	},
	{ // case 11
		node:  &SizeOf{X: &Ident{Name: "var4"}},
		value: int64(11),
		kind:  reflect.Int64,
	},
	{ // case 12
		node:  &SizeOf{X: &Ident{Name: "var5"}},
		value: int64(2),
		kind:  reflect.Int64,
	},
	{ // case 13
		node:  &SizeOf{X: &ArrayLiteral{Values: []Node{il1, il2, fl2, sl2}}},
		value: int64(4),
		kind:  reflect.Int64,
	},
	{ // case 14
		node:  &SizeOf{X: &MapLiteral{Keys: []Node{fl1, il1, sl1}, Values: []Node{il1, sl1, il2}}},
		value: int64(3),
		kind:  reflect.Int64,
	},
	{ // case 15
		node:  &IsType{X: il1, Types: []token.Token{token.TFLOAT, token.TMAP}},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 16
		node:  &IsType{X: il1, Types: []token.Token{token.TFLOAT, token.TINTEGER}},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 17
		node:  &IsType{X: &Ident{Name: "var4"}, Types: []token.Token{token.TFLOAT, token.TINTEGER}},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 18
		node:  &IsType{X: &Ident{Name: "var2"}, Types: []token.Token{token.TFLOAT, token.TINTEGER}},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 19, ignore, type check does not look for value
		node:  &IsType{X: &ArrayLiteral{}, Types: []token.Token{token.TFLOAT, token.TARRAY}},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 20, ignore, type check does not look for value
		node:  &IsType{X: &MapLiteral{}, Types: []token.Token{token.TMAP, token.TARRAY}},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 21, ignore, type check does not look for value
		node:  &IsType{X: &MapLiteral{}, Types: []token.Token{token.TINTEGER, token.TARRAY}},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 22
		node:  &TypeCast{X: bl1, To: token.STRING},
		value: bl1.Lit,
		kind:  reflect.String,
	},
	{ // case 23
		node:  &TypeCast{X: sl1, To: token.FLOAT},
		value: 98.0,
		kind:  reflect.Float64,
	},
	{ // case 24
		node:  &ArrayLiteral{Values: []Node{il1, il2, bl2}},
		value: []any{int64(12), int64(21), false},
		kind:  reflect.Slice,
	},
	{ // case 25
		node:  &MapLiteral{Keys: []Node{sl1, sl2, fl1}, Values: []Node{il1, il2, bl1}},
		value: map[any]any{sl1.Lit: int64(12), sl2.Lit: int64(21), 5.2: true},
		kind:  reflect.Map,
	},
	{ // case 26
		node: wrapExpr(
			&Delete{X: &Ident{Name: "var7"}, Indexes: indexesNode(0, 2, 3)},
			&Ident{Name: "var7"},
		),
		value: []any{int64(21), true, false, "98", "sample"},
		kind:  reflect.Slice,
	},
	{ // case 27
		node: wrapExpr(
			&Delete{X: &Ident{Name: "var7"}, Indexes: indexesNode(1), End: indexesNode(3)[0]},
			&Ident{Name: "var7"},
		),
		value: []any{int64(21), "sample"},
		kind:  reflect.Slice,
	},
	{ // case 28
		node: wrapExpr(
			&Delete{X: &Ident{Name: "var8"}, Indexes: indexesNode(1, 3, 7)},
			&Ident{Name: "var8"},
		),
		value: map[any]any{int64(2): int64(6), int64(4): int64(10), int64(5): int64(12), int64(6): int64(14), int64(8): int64(18)},
		kind:  reflect.Map,
	},
	{ // case 29
		node: wrapExpr(
			&Delete{X: &Ident{Name: "var8"}, Indexes: indexesNode(2)},
			&Ident{Name: "var8"},
		),
		value: map[any]any{int64(4): int64(10), int64(5): int64(12), int64(6): int64(14), int64(8): int64(18)},
		kind:  reflect.Map,
	},
	{ // case 30
		node:  &Index{X: &Ident{Name: "var8"}, Index: indexesNode(5)[0]},
		value: int64(12),
		kind:  reflect.Int64,
	},
	{ // case 31
		node:  &Index{X: &Ident{Name: "var7"}, Index: indexesNode(1)[0]},
		value: "sample",
		kind:  reflect.String,
	},
	{ // case 32
		node:  &SubValue{X: &BasicLit{Lit: "sample text", Kind: token.STRING}, Range: &Interval{A: indexesNode(0)[0], B: indexesNode(7)[0]}},
		value: "ample",
		kind:  reflect.String,
	},
	{ // case 33
		node:  &SubValue{X: &ArrayLiteral{Values: indexesNode(1, 2, 3, 4, 5, 6, 7)}, Range: &Interval{A: indexesNode(0)[0], B: indexesNode(5)[0]}},
		value: []any{int64(2), int64(3), int64(4)},
		kind:  reflect.Slice,
	},
	{ // case 34
		node:  &Interval{A: indexesNode(1)[0], AInclude: true, B: indexesNode(10)[0], BInclude: true},
		value: []int64{1, 10, 1},
		kind:  reflect.Slice,
	},
	{ // case 35
		node:  &Interval{A: indexesNode(1)[0], B: indexesNode(10)[0], BInclude: true},
		value: []int64{2, 10, 1},
		kind:  reflect.Slice,
	},
	{ // case 36
		node:  &Interval{A: fl1, B: fl2, BInclude: true, Step: &BasicLit{Lit: "3", Kind: token.INTEGER}},
		value: []float64{8.2, 29.1, 3},
		kind:  reflect.Slice,
	},
	{ // case 37
		node:  &Interval{A: fl1, B: sl1, BInclude: true, Step: &BasicLit{Lit: "6", Kind: token.INTEGER}},
		value: []float64{11.2, 98.0, 6},
		kind:  reflect.Slice,
	},
	{ // case 38
		node:  &Interval{A: bl1, B: bl2, Step: sl2},
		isErr: true,
	},
	{ // case 39
		node:  &Interval{A: bl1, B: bl2, Step: sl1},
		isErr: true,
	},
	{ // case 40
		node:  &Interval{A: il1, B: bl2, Step: sl1},
		isErr: true,
	},
	{ // case 41
		node:  &Unary{X: bl1, Op: token.NOT},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 42
		node:  &Unary{X: il1, Op: token.ADD},
		value: int64(12),
		kind:  reflect.Int64,
	},
	{ // case 43
		node:  &Unary{X: sl1, Op: token.SUB},
		value: -int64(98),
		kind:  reflect.Int64,
	},
	{ // case 44
		node:  &Unary{X: il1, Op: token.XOR},
		value: ^int64(12),
		kind:  reflect.Int64,
	},
	{ // case 45
		node:  &Unary{X: bl1, Op: token.NOT},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 46
		node:  &Unary{X: il1, Op: token.NOT},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 47
		node:  &IncDec{X: il1, Op: token.INC},
		value: int64(13),
		kind:  reflect.Int64,
	},
	{ // case 48
		node:  &IncDec{X: fl1, Op: token.DEC},
		value: 4.2,
		kind:  reflect.Float64,
	},
	{ // case 49
		node:  &Binary{L: il1, Op: token.ADD, R: fl1},
		value: 17.2,
		kind:  reflect.Float64,
	},
	{ // case 50
		node:  &Binary{L: il1, Op: token.ADD, R: il2},
		value: int64(33),
		kind:  reflect.Int64,
	},
	{ // case 51
		node:  &Binary{L: il1, Op: token.GEQ, R: il2},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 52
		node:  &Binary{L: il2, Op: token.GEQ, R: fl1},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 53
		node:  &Binary{L: sl1, Op: token.GEQ, R: fl1},
		isErr: true,
	},
	{ // case 54
		node:  &Binary{L: bl1, Op: token.ADD, R: sl1},
		value: "true98",
		kind:  reflect.String,
	},
	{ // case 55
		node:  &Binary{L: bl1, Op: token.MUL, R: sl1},
		isErr: true,
	},
	{ // case 56, 12 * 29.1 + 5.2
		node:  &Binary{L: &Binary{L: il1, Op: token.MUL, R: fl2}, Op: token.ADD, R: fl1},
		value: (float64(12) * 29.1) + 5.2,
		kind:  reflect.Float64,
	},
	{ // case 57
		node:  &Binary{L: bl1, Op: token.LAND, R: &Binary{L: il1, Op: token.GTR, R: fl2}},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 58
		node:  &SizeOf{X: &Unary{Op: token.FD, X: &BasicLit{Lit: "basic.go", Kind: token.STRING}}},
		value: basicSize,
		kind:  reflect.Int64,
	},
	{ // case 59
		node:  &SizeOf{X: &Unary{Op: token.FD, X: &BasicLit{Lit: "nowhere", Kind: token.STRING}}},
		value: int64(-1),
		kind:  reflect.Int64,
	},
	{ // case 60
		node:  &SizeOf{X: &Unary{Op: token.FD, X: &BasicLit{Lit: filepath.Join("..", "parser"), Kind: token.STRING}}},
		value: int64(4),
		kind:  reflect.Int64,
	},
	{ // case 61
		node: &Pipe{
			X: &Call{Kind: token.AT, Name: "print", Args: []Node{echo, &BasicLit{Lit: "text", Kind: token.STRING}}},
			Y: &Pipe{
				X: &Call{Kind: token.AT, Name: "print", Args: []Node{echo, &BasicLit{Lit: "121", Kind: token.INTEGER}}},
				Y: &Call{Kind: token.AT, Name: "print", Args: []Node{echo, &BasicLit{Lit: "9.1", Kind: token.FLOAT}}},
			},
		},
		value: "9.1 121 text\n\n\n",
		kind:  reflect.String,
	},
	{ // case 62
		node: &Pipe{
			X: &Call{Kind: token.AT, Name: "print", Args: []Node{echo, &BasicLit{Lit: "text", Kind: token.STRING}}},
			Y: &Call{Kind: token.AT, Name: "print", Args: []Node{echo, &BasicLit{Lit: "3915", Kind: token.INTEGER}}},
		},
		value: "3915 text\n\n",
		kind:  reflect.String,
	},
	{ // case 63
		node:  &OSysCheck{OS: curOSTok},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 64
		node:  &OSysCheck{OS: nonOSTok},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 65
		node:  &Exists{Op: token.AT, X: &Ident{Name: "print"}},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 66
		node:  &Exists{Op: token.HASH, X: &Ident{Name: "rmdir"}}, // rmdir command is exist on both unix and windows
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 67
		node:  &Exists{X: &Ident{Name: "sample"}}, // rmdir command is exist on both unix and windows
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 68
		node:  &Exists{Op: token.FD, X: &BasicLit{Lit: "_____", Kind: token.STRING}},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 69
		node:  &Exists{Op: token.FD, X: &BasicLit{Lit: "basic.go", Kind: token.STRING}},
		value: true,
		kind:  reflect.Bool,
	},
	{ // case 70
		node:  &Exists{X: &Index{Base: dummyBase, Index: &BasicLit{Lit: "10", Kind: token.INTEGER}, X: &Ident{Name: "var5"}}},
		value: false,
		kind:  reflect.Bool,
	},
	{ // case 71
		node:  &Exists{X: &Index{Base: dummyBase, Index: &BasicLit{Lit: "119.0", Kind: token.INTEGER}, X: &Ident{Name: "var6"}}},
		value: false,
		kind:  reflect.Bool,
	},
}

func TestExpression(t *testing.T) {
	// exprCases is initialize before init function therefore basicSize is always 0
	exprCases[57].value = basicSize
	for i, tc := range exprCases {
		t.Logf("TestBasicLit case #%d", i+1)
		v, k, err := tc.node.Evaluate(ctx)
		if tc.isErr {
			assert.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tc.value, v)
			assert.Equal(t, tc.kind, k)
		}
	}
}

func TestCallExpression(t *testing.T) {
	// test call external command
	call := &Call{Kind: token.HASH, Name: "go", Args: []Node{&BasicLit{Lit: "version", Kind: token.STRING}}, OutputResult: true}
	result, k, err := call.Evaluate(ctx)
	require.NoError(t, err)
	assert.Equal(t, k, reflect.String)
	assert.Equal(t, fmt.Sprintf("go version %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH), result)
	// test target
	icook := NewCook()
	target, err := icook.AddTarget(dummyBase, "sample")
	require.NoError(t, err)
	target.Insts.Append(&AssignStatement{
		Ident: &Ident{Name: "sample"},
		Op:    token.ASSIGN,
		Value: &Binary{L: il1, Op: token.MUL, R: &Binary{L: fl2, Op: token.ADD, R: &Ident{Name: "1"}}},
	})
	ctx := icook.(*cook).renewContext()
	ctx.SetVariable("sample", int64(5), reflect.Int64, nil)
	call = &Call{Kind: token.AT, Name: "sample", Args: indexesNode(22)}
	result, k, err = call.Evaluate(ctx)
	require.NoError(t, err)
	assert.Equal(t, reflect.Invalid, k)
	assert.Equal(t, nil, result)
	// check sample variable
	result, k, _ = ctx.GetVariable("sample")
	assert.Equal(t, reflect.Float64, k)
	assert.Equal(t, float64(12)*(29.1+22), result)
	// test command built-in
	call = &Call{Kind: token.AT, Name: "print", Args: []Node{&BasicLit{Lit: "-e", Kind: token.STRING}, sl2}}
	result, k, err = call.Evaluate(ctx)
	require.NoError(t, err)
	assert.Equal(t, reflect.String, k)
	assert.Equal(t, sl2.Lit+"\n", result)
	// test function literal
	idents := []*Ident{{Name: "a"}, {Name: "b"}}
	call = &Call{
		Kind: token.AT,
		FuncLit: &Function{
			Args: idents,
			Insts: &BlockStatement{
				Stmts: []Statement{
					&ReturnStatement{
						X: &Binary{L: idents[0], Op: token.MUL, R: &Conditional{
							Cond:  &Binary{L: idents[1], Op: token.GEQ, R: il1},
							True:  idents[1],
							False: fl1,
						}},
					},
				},
			},
		},
	}
	// case 1
	call.Args = indexesNode(2, 22)
	result, k, err = call.Evaluate(ctx)
	require.NoError(t, err)
	assert.Equal(t, reflect.Int64, k)
	assert.Equal(t, int64(2*22), result)
	// case 2
	call.Args = indexesNode(2, 8)
	result, k, err = call.Evaluate(ctx)
	require.NoError(t, err)
	assert.Equal(t, reflect.Float64, k)
	assert.Equal(t, 2*5.2, result)
	// case 3
	call.Args = indexesNode(2, 10)
	result, k, err = call.Evaluate(ctx)
	require.NoError(t, err)
	assert.Equal(t, reflect.Float64, k)
	assert.Equal(t, 2*5.2, result)
}

func verifyContentFile(t *testing.T, file, content string) {
	require.FileExists(t, file)
	b, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, content, string(b))
}

func TestCallExprRedirect(t *testing.T) {
	file1 := "sample1.txt"
	file2 := "sample2.txt"
	content := "sample text"
	defer os.Remove(file1)
	defer os.Remove(file2)
	redirect := &RedirectTo{
		Append: false,
		Files: []Node{
			&BasicLit{Lit: file1, Kind: token.STRING},
			&BasicLit{Lit: file2, Kind: token.STRING},
		},
		Caller: &Call{
			Name: "print",
			Kind: token.AT,
			Args: []Node{
				&BasicLit{Lit: "-e", Kind: token.STRING},
				&BasicLit{Lit: content, Kind: token.STRING},
			},
		},
	}
	ctx := NewCook().(*cook).renewContext()
	r, k, err := redirect.Evaluate(ctx)
	require.NoError(t, err)
	assert.Equal(t, reflect.Invalid, k)
	assert.Equal(t, nil, r)
	// check file and it content
	verifyContentFile(t, file1, content+"\n")
	verifyContentFile(t, file2, content+"\n")
	// try append
	redirect.Append = true
	redirect.Files = redirect.Files[:1]
	_, _, err = redirect.Evaluate(ctx)
	require.NoError(t, err)
	verifyContentFile(t, file1, content+"\n"+content+"\n")
	verifyContentFile(t, file2, content+"\n")
	// call read from
	call := &Call{
		Kind: token.AT,
		FuncLit: &Function{
			Args: []*Ident{{Name: "a"}},
			Insts: &BlockStatement{
				Stmts: []Statement{
					&AssignStatement{
						Ident: &Ident{Name: "a"},
						Op:    token.ADD_ASSIGN,
						Value: &BasicLit{Lit: "new text", Kind: token.STRING},
					},
					&ReturnStatement{X: &Ident{Name: "a"}},
				},
			},
		},
		Args: []Node{&ReadFrom{File: &BasicLit{Lit: file2, Kind: token.STRING}}},
	}
	r, k, err = call.Evaluate(ctx)
	require.NoError(t, err)
	assert.Equal(t, reflect.String, k)
	assert.Equal(t, content+"\nnew text", r)
}

func TestTransformation(t *testing.T) {
	numElem := 10
	c := NewCook().(*cook)
	c.renewContext()
	// add array and map
	a := make([]any, 10)
	b := make(map[any]any)
	for i := 0; i < numElem; i++ {
		a[int64(i)] = int64(i + 1)
		b[int64(i)] = int64(i + 1)
	}
	// globle variable
	avar := &Ident{Name: "a"}
	bvar := &Ident{Name: "b"}
	cvar := &Ident{Name: "c"}
	dvar := &Ident{Name: "d"}
	evar := &Ident{Name: "e"}
	fvar := &Ident{Name: "f"}
	// variable argument
	iiden := &Ident{Name: "i"}
	kiden := &Ident{Name: "k"}
	viden := &Ident{Name: "v"}
	// add block code
	c.Block().Append(&AssignStatement{
		Ident: cvar,
		Op:    token.ASSIGN,
		Value: &Transformation{
			Ident: avar,
			Fn: &Function{
				Lambda: token.LAMBDA,
				Args:   []*Ident{iiden, viden},
				X: &Binary{
					L:  viden,
					Op: token.MUL,
					R:  &Binary{L: iiden, Op: token.ADD, R: &BasicLit{Lit: "1.5", Kind: token.FLOAT}},
				},
			},
		},
	})
	c.Block().Append(&AssignStatement{
		Ident: evar,
		Op:    token.ASSIGN,
		Value: &Transformation{
			Ident: cvar,
			Fn: &Function{
				Lambda: token.LAMBDA,
				Args:   []*Ident{iiden, viden},
				X: &Binary{
					L:  &Binary{L: viden, Op: token.ADD, R: &BasicLit{Lit: "4", Kind: token.INTEGER}},
					Op: token.MUL,
					R:  &Binary{L: iiden, Op: token.ADD, R: &BasicLit{Lit: "3", Kind: token.FLOAT}},
				},
			},
		},
	})
	c.Block().Append(&AssignStatement{
		Ident: dvar,
		Op:    token.ASSIGN,
		Value: &Transformation{
			Ident: bvar,
			Fn: &Function{
				Lambda: token.LAMBDA,
				Args:   []*Ident{kiden, viden},
				X: &Binary{
					L:  viden,
					Op: token.MUL,
					R:  &Binary{L: kiden, Op: token.MUL, R: &BasicLit{Lit: "3", Kind: token.INTEGER}},
				},
			},
		},
	})
	c.Block().Append(&AssignStatement{
		Ident: fvar,
		Op:    token.ASSIGN,
		Value: &Transformation{
			Ident: dvar,
			Fn: &Function{
				Lambda: token.LAMBDA,
				Args:   []*Ident{kiden, viden},
				X: &Binary{
					L:  &Binary{L: viden, Op: token.MUL, R: &BasicLit{Lit: "2", Kind: token.INTEGER}},
					Op: token.MUL,
					R:  &Binary{L: kiden, Op: token.MUL, R: &BasicLit{Lit: "3", Kind: token.INTEGER}},
				},
			},
		},
	})
	c.AddTarget(dummyBase, "all")
	require.NoError(t, c.Execute(map[string]any{
		avar.Name: a,
		bvar.Name: b,
	}))
	alter := false
	vcfn := func(i int) float64 { return float64(i + 1) }
	vdfn := func(i int) int64 { return int64(i + 1) }
retest:
	for _, ident := range []*Ident{cvar, evar} {
		for i := 0; i < numElem; i++ {
			t.Logf("Verify array value by index #%d", i)
			index := &Index{
				Index: &BasicLit{Lit: strconv.Itoa(i), Kind: token.INTEGER},
				X:     ident,
			}
			result, kind, err := index.Evaluate(c.ctx)
			require.NoError(t, err)
			assert.Equal(t, reflect.Float64, kind)
			v := vcfn(i)
			ev := v * (float64(i) + 1.5)
			if ident == cvar {
				assert.Equal(t, ev, result)
			} else {
				assert.Equal(t, (ev+4)*(float64(i)+3), result)
			}
		}
	}
	for _, ident := range []*Ident{dvar, fvar} {
		for i := 0; i < numElem; i++ {
			t.Logf("Verify map value by key #%d", i)
			index := &Index{
				Index: &BasicLit{Lit: strconv.Itoa(i), Kind: token.INTEGER},
				X:     ident,
			}
			result, kind, err := index.Evaluate(c.ctx)
			require.NoError(t, err)
			assert.Equal(t, reflect.Int64, kind)
			v := vdfn(i)
			ev := v * (int64(i) * 3)
			if ident == dvar {
				assert.Equal(t, ev, result)
			} else {
				assert.Equal(t, (ev*2)*(int64(i)*3), result)
			}
		}
	}
	if !alter {
		alter = true
		for i := 0; i < numElem; i++ {
			a[int64(i)] = int64(i+1) * 2
			b[int64(i)] = int64(i+1) * 3
		}
		vcfn = func(i int) float64 { return float64(i+1) * 2 }
		vdfn = func(i int) int64 { return int64(i+1) * 3 }
		goto retest
	}
}

func TestStringInterpolation(t *testing.T) {
	a := "interpolation"
	b := 3.5
	out := fmt.Sprintf("sample %s text, expression 82 & 3 * b %g is allow to be include in string %s as well.", a, 82/(3*b), a)
	sib := NewStringInterpolationBuilder('\'')
	sib.WriteString("sample ")
	sib.AddExpression(&Ident{Name: "a"})
	sib.WriteString(" text, expression 82 & 3 * b ")
	sib.AddExpression(&Binary{
		L:  &BasicLit{Lit: "82", Kind: token.INTEGER},
		Op: token.QUO,
		R:  &Binary{L: &BasicLit{Lit: "3", Kind: token.INTEGER}, Op: token.MUL, R: &Ident{Name: "b"}},
	})
	sib.WriteString(" is allow to be include in string ")
	sib.AddExpression(&Ident{Name: "a"})
	sib.WriteString(" as well.")
	ctx := NewCook().(*cook).renewContext()
	ctx.SetVariable("a", "interpolation", reflect.String, nil)
	ctx.SetVariable("b", 3.5, reflect.Float64, nil)
	si := sib.Build(0, token.NewFile("sample", 0))
	v, k, err := si.Evaluate(ctx)
	require.NoError(t, err)
	assert.Equal(t, reflect.String, k)
	assert.Equal(t, out, v)
}
