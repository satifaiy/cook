package ast

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"

	"github.com/cozees/cook/pkg/cook/token"
	cookErrors "github.com/cozees/cook/pkg/errors"
	"github.com/cozees/cook/pkg/runtime/args"
	"github.com/cozees/cook/pkg/runtime/function"
)

type Node interface {
	Code
	ErrPos() string
	Position() token.Position
	Evaluate(ctx Context) (any, reflect.Kind, error)
}

type SettableNode interface {
	Node
	VariableName() string
	Set(ctx Context, v any, k reflect.Kind, bubble func(v any, k reflect.Kind) error) error
	IsEqual(sn SettableNode) bool
}

type (
	// provide basic info related to ast node
	Base struct {
		Offset int
		File   *token.File
	}

	// A node represent literal value
	BasicLit struct {
		*Base
		Kind token.Token
		Mark byte
		Lit  string
	}

	// A node represent a variable
	Ident struct {
		*Base
		Name string
	}

	// A node represent short if or ternary expression
	Conditional struct {
		*Base
		Cond  Node
		True  Node
		False Node
	}

	// A node represent fallback expression
	Fallback struct {
		*Base
		Primary Node
		Default Node
	}

	// A node represent size of a variable or literal value expression
	SizeOf struct {
		*Base
		X Node
	}

	// A node represent type check expression
	IsType struct {
		*Base
		X     Node
		Types []token.Token
	}

	// A node represent type cast expression
	TypeCast struct {
		*Base
		X  Node
		To token.Token
	}

	// A node represent exit expression statement
	Exit struct {
		*Base
		ExitCode Node
	}

	// A node represent array or list literal
	ArrayLiteral struct {
		*Base
		Multiline bool
		Values    []Node
	}

	// A node represent map literal
	MapLiteral struct {
		*Base
		Multiline bool
		Keys      []Node
		Values    []Node
	}

	// A new represent map merge operation
	// A += < {...} or A += ? {...}
	MergeMap struct {
		*Base
		Op    token.Token // must be < or ?
		Value Node        // either a variable holding map or a map literal
	}

	// A node represent delete statement
	Delete struct {
		*Base
		Indexes []Node
		End     Node // if End is not nil, Var must be array and it's delete range.
		X       *Ident
	}

	// A node represent index expression
	Index struct {
		*Base
		Index Node
		X     Node
	}

	// A node represent sub value of original value
	SubValue struct {
		*Base
		Range *Interval
		X     Node
	}

	// A node represent range value
	Interval struct {
		*Base
		A        Node
		AInclude bool
		B        Node
		BInclude bool
		Step     Node
	}

	// A node represent expression to check operating system
	OSysCheck struct {
		*Base
		OS token.Token
	}

	// A node represent expression to check something is existed include
	// 1. Variable
	// 2. File or Directory
	// 3. Command
	// 4. Function
	Exists struct {
		*Base
		X  Node
		Op token.Token
	}

	// A node represent call expression
	Call struct {
		*Base
		Kind         token.Token
		Args         []Node
		Name         string
		OutputResult bool
		FuncLit      *Function

		// use internally for share argument with pipe expression
		pipeCmdArgs     string
		pipeBuiltInArgs *args.FunctionArg
	}

	// A node represent pipe expression
	Pipe struct {
		*Base
		X *Call
		Y Node
	}

	// A node represent redirect read file expression <
	ReadFrom struct {
		*Base
		File Node
	}

	// A node represent redirect write to or append to one or more file expression
	RedirectTo struct {
		*Base
		Files  []Node
		Caller Node
		Append bool
	}

	// A node represent parenthese expression
	Paren struct {
		*Base
		Inner Node
	}

	// A node represent unary expression
	Unary struct {
		*Base
		Op token.Token
		X  Node
	}

	// A node represent incremenet or decrement expression
	IncDec struct {
		*Base
		Op token.Token
		X  Node
	}

	// A node represent binary tree operator
	Binary struct {
		*Base
		L  Node
		Op token.Token
		R  Node
	}

	// A node represent a transform expression
	Transformation struct {
		*Base
		Ident SettableNode
		Fn    *Function
		to    SettableNode // use by assign statement
	}
)

func (b *Base) Position() token.Position { return b.File.Position(b.Offset) }
func (b *Base) ErrPos() string {
	p := b.Position()
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}

// BasicLit Evaluate convert a string lit value to it corresponding type value such as
// integer, float, bool ...etc
func (bl *BasicLit) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	switch bl.Kind {
	case token.INTEGER:
		v, err = strconv.ParseInt(bl.Lit, 10, 64)
		k = reflect.Int64
	case token.FLOAT:
		v, err = strconv.ParseFloat(bl.Lit, 64)
		k = reflect.Float64
	case token.BOOLEAN:
		v, err = strconv.ParseBool(bl.Lit)
		k = reflect.Bool
	case token.STRING:
		v, k = bl.Lit, reflect.String
	default:
		return nil, 0, fmt.Errorf("%s: invalid literal value %s of type %s", bl.ErrPos(), bl.Lit, bl.Kind)
	}
	if err != nil {
		return nil, 0, err
	} else {
		return v, k, nil
	}
}

// Ident Evaluate read value from current scope always to global scope. If no variable exist, it will
// look into environment varaible and return it value if founded otherwise a nil and invalid kind is
// return instead.
func (id *Ident) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	v, k, _ = ctx.GetVariable(id.Name)
	return
}

func (id *Ident) VariableName() string { return id.Name }

func (id *Ident) Set(ctx Context, v any, k reflect.Kind, bubble func(v any, k reflect.Kind) error) (err error) {
	if v != nil {
		ctx.SetVariable(id.Name, v, k, bubble)
	} else {
		err = fmt.Errorf("assign nil value to variable %s", id.Name)
	}
	return
}

func (id *Ident) IsEqual(sn SettableNode) bool {
	pid, ok := sn.(*Ident)
	return ok && id.Name == pid.Name
}

// Conditional Evaluate check the condition boolean result, if the result is true is return result of
// True expression's evaluation otherwise a result from False expression's evalute is return instread.
// This expression is known as short if or ternary expression.
func (cd *Conditional) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	if v, k, err = cd.Cond.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else if k != reflect.Bool {
		return nil, 0, fmt.Errorf("%s: expression %s is not a valid boolean expression", cd.Cond.ErrPos(), cd.Cond)
	} else if v.(bool) {
		return cd.True.Evaluate(ctx)
	} else {
		return cd.False.Evaluate(ctx)
	}
}

// Fallback Evaluate return the primary result if no error
func (fb *Fallback) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	if v, k, err = fb.Primary.Evaluate(ctx); err != nil || v == nil {
		pErr := err
		v, k, err = fb.Default.Evaluate(ctx)
		if err != nil {
			ce := &cookErrors.CookError{}
			if pErr != nil {
				ce.StackError(fmt.Errorf("primary error %w", pErr))
			} else {
				ce.StackError(fmt.Errorf("primary value nil"))
			}
			ce.StackError(fmt.Errorf(", fallback error %w", err))
			return nil, 0, ce
		}
	}
	return
}

// SizeOf Evaluate return size of variable or literal value such as string, array, map
func (sf *SizeOf) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	if unary, ok := sf.X.(*Unary); ok && unary.Op == token.FD {
		// return the size of the file instead
		if fp, k, err := unary.Evaluate(ctx); err != nil {
			return nil, reflect.Invalid, err
		} else if k != reflect.String {
			return nil, reflect.Invalid, fmt.Errorf("%s is valid string filepath", unary)
		} else if stat, err := os.Stat(fp.(string)); err != nil {
			return int64(-1), reflect.Int64, nil
		} else if stat.IsDir() {
			if fis, err := os.ReadDir(fp.(string)); err != nil {
				return nil, reflect.Invalid, err
			} else {
				return int64(len(fis)), reflect.Int64, nil
			}
		} else {
			return stat.Size(), reflect.Int64, nil
		}
	} else if v, k, err = sf.X.Evaluate(ctx); err != nil {
		return nil, 0, err
	}
	switch k {
	case reflect.Slice, reflect.Map, reflect.String:
		v = int64(reflect.ValueOf(v).Len())
	default:
		return nil, 0, fmt.Errorf("sizeof not supporting value %v", v)
	}
	return v, reflect.Int64, nil
}

// IsType Evaluate return a boolean value. It return true if variable X value is satified
// the given type Token.
func (it *IsType) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	bit := 0
	for _, tok := range it.Types {
		bit |= tok.Type()
	}
	if bl, ok := it.X.(*BasicLit); ok {
		ttok := bl.Kind + token.TINTEGER - token.INTEGER
		if ttok < token.TINTEGER || ttok > token.TSTRING {
			panic("BasicLit expression only supoort integer, float, boolean and string")
		}
		kbit := ttok.Type()
		v = bit&kbit == kbit
		k = reflect.Bool
	} else if _, ok = it.X.(*ArrayLiteral); ok {
		kbit := token.TARRAY.Type()
		return bit&kbit == kbit, reflect.Bool, nil
	} else if _, ok = it.X.(*MapLiteral); ok {
		kbit := token.TMAP.Type()
		return bit&kbit == kbit, reflect.Bool, nil
	} else if _, ok := it.X.(*Ident); ok {
		var kbit int
		switch _, k, _ = it.X.Evaluate(ctx); k {
		case reflect.Int64:
			kbit = token.TINTEGER.Type()
		case reflect.Float64:
			kbit = token.TFLOAT.Type()
		case reflect.String:
			kbit = token.TSTRING.Type()
		case reflect.Bool:
			kbit = token.TBOOLEAN.Type()
		case reflect.Array, reflect.Slice:
			kbit = token.TARRAY.Type()
		case reflect.Map:
			kbit = token.TMAP.Type()
		default:
			kbit = token.TOBJECT.Type()
		}
		v = bit&kbit == kbit
		k = reflect.Bool
	} else {
		return nil, 0, fmt.Errorf("%s of %s must be a literal value or variable", it, it.X)
	}
	return
}

// TypeCast Evaluate return convertible value converted from string to string
func (tc *TypeCast) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	iv, ik, _ := tc.X.Evaluate(ctx)
	tk := tc.To.Kind()
	if tk != ik {
		switch tk {
		case reflect.Int64:
			if ik == reflect.Float64 {
				return int64(iv.(float64)), tk, nil
			} else if k == reflect.String {
				if v, err = strconv.ParseInt(iv.(string), 10, 64); err == nil {
					k = tk
					return
				}
			}
		case reflect.Float64:
			if ik == reflect.Int64 {
				return float64(iv.(int64)), tk, nil
			} else if ik == reflect.String {
				if v, err = strconv.ParseFloat(iv.(string), 64); err == nil {
					k = tk
					return
				}
			}
		case reflect.Bool:
			if ik == reflect.String {
				if v, err = strconv.ParseBool(iv.(string)); err == nil {
					k = tk
					return
				}
			}
		case reflect.String:
			if ik == reflect.Int64 {
				return strconv.FormatInt(iv.(int64), 10), tk, nil
			} else if ik == reflect.Float64 {
				return strconv.FormatFloat(iv.(float64), 'g', -1, 64), tk, nil
			} else if ik == reflect.Bool {
				return strconv.FormatBool(iv.(bool)), tk, nil
			}
		}
		err = fmt.Errorf("%s cannot cast %v to type %s", tc.ErrPos(), iv, tk)
	} else {
		v, k = iv, ik
	}
	return
}

// Exit Evaluate will exit the execution with the given code.
func (e *Exit) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	if v, r, err := e.ExitCode.Evaluate(ctx); err == nil {
		var code int64
		switch r {
		case reflect.Int64:
			code = v.(int64)
		case reflect.String:
			if code, err = strconv.ParseInt(v.(string), 10, 64); err != nil {
				return nil, 0, err
			}
		default:
			return nil, 0, fmt.Errorf("exit code must an integer")
		}
		os.Exit(int(code))
	}
	return nil, 0, err
}

// ArrayLiteral Evaluate return value of array or list
func (al *ArrayLiteral) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	result := make([]any, 0, len(al.Values))
	for _, lv := range al.Values {
		if v, _, err = lv.Evaluate(ctx); err != nil {
			return nil, 0, err
		}
		result = append(result, v)
	}
	return result, reflect.Slice, nil
}

// MapLiteral Evaluate return value of map or directionary
func (ml *MapLiteral) Evaluate(ctx Context) (any, reflect.Kind, error) {
	m := make(map[any]any)
	for i, lk := range ml.Keys {
		vk, _, err := lk.Evaluate(ctx)
		if err != nil {
			return nil, 0, err
		}
		vv, _, err := ml.Values[i].Evaluate(ctx)
		if err != nil {
			return nil, 0, err
		}
		m[vk] = vv
	}
	return m, reflect.Map, nil
}

func (mm *MergeMap) Evaluate(ctx Context) (any, reflect.Kind, error) {
	return nil, 0, errors.New("merge map expression does not support evaluate, it indent to use as Set")
}

func (mm *MergeMap) VariableName() string { return mm.Value.String() }

func (mm *MergeMap) Set(ctx Context, setVal any, _ reflect.Kind) error {
	vv := reflect.ValueOf(setVal)
	if vv.Kind() != reflect.Map {
		return fmt.Errorf("%v is not a map", setVal)
	}
	v, k, err := mm.Value.Evaluate(ctx)
	if err != nil {
		return err
	} else if k != reflect.Map {
		return fmt.Errorf("%s is not a map or it's value is not a map", mm.Value)
	}
	setv := reflect.ValueOf(v)
	mitem := setv.MapRange()
	for mitem.Next() {
		key, val := mitem.Key(), mitem.Value()
		if mm.Op == token.LSS {
			vv.SetMapIndex(key, val)
		} else if (vv.MapIndex(key) == reflect.Value{}) {
			// key not exist
			vv.SetMapIndex(key, val)
		} else if mm.Op != token.QES {
			return fmt.Errorf("map: key %v is already exist, use '?' to ignore the error", key.Interface())
		}
	}
	return nil
}

// Delete Evaluate delete item from array or map. Delete return error if there is an error occorred
// otherwise the result value is always nil
func (d *Delete) Evaluate(ctx Context) (any, reflect.Kind, error) {
	if v, k, err := d.X.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else {
		switch k {
		case reflect.Slice:
			// delete by range
			if d.End != nil {
				if se, err := indexes(ctx, d.Indexes[0], d.End); err != nil {
					return nil, 0, err
				} else {
					vv := reflect.ValueOf(v)
					if se[0] >= 0 && se[1] < vv.Len() {
						for i := se[1]; i >= se[0]; i-- {
							vv = reflect.AppendSlice(vv.Slice(0, i), vv.Slice(i+1, vv.Len()))
						}
					} else {
						return nil, 0, fmt.Errorf("range %d..%d of out range array length %d", se[0], se[1], vv.Len())
					}
					ctx.SetVariable(d.X.Name, vv.Interface(), k, nil)
				}
			} else if ids, err := indexes(ctx, d.Indexes...); err != nil {
				return nil, 0, err
			} else {
				vv := reflect.ValueOf(v)
				if ids[0] < 0 || ids[len(ids)-1] >= vv.Len() {
					return nil, 0, fmt.Errorf("delete indexes out range, array length %d", vv.Len())
				}
				for i := range ids {
					ri := len(ids) - 1 - i
					vv = reflect.AppendSlice(vv.Slice(0, ids[ri]), vv.Slice(ids[ri]+1, vv.Len()))
				}
				ctx.SetVariable(d.X.Name, vv.Interface(), k, nil)
			}
		case reflect.Map:
			if d.End != nil {
				// parser must ensure map does not support range delete
				panic("delete map with range")
			}
			vv := reflect.ValueOf(v)
			for _, k := range d.Indexes {
				if kd, _, err := k.Evaluate(ctx); err != nil {
					return nil, 0, err
				} else {
					vv.SetMapIndex(reflect.ValueOf(kd), reflect.Value{})
				}
			}
		default:
			return nil, 0, fmt.Errorf("only array or map support delete")
		}
	}
	return nil, 0, nil
}

// Index Evaluate return item of array or map
func (ix *Index) Evaluate(ctx Context) (any, reflect.Kind, error) {
	return ix.evaluateInternal(ctx, nil)
}

func (ix *Index) VariableName() string { return ix.X.String() }

func (ix *Index) Set(ctx Context, v any, k reflect.Kind, _ func(v any, k reflect.Kind) error) (err error) {
	_, _, err = ix.evaluateInternal(ctx, v)
	return
}

func (ix *Index) IsEqual(sn SettableNode) bool {
	if pix, ok := sn.(*Index); !ok {
		return false
	} else if pid, pok := ix.X.(*Ident); !pok {
		return false
	} else if pixid, ixok := pix.X.(*Ident); !ixok {
		return false
	} else if !pixid.IsEqual(pid) {
		return false
	} else {
		bl1, ok1 := ix.Index.(*BasicLit)
		bl2, ok2 := pix.Index.(*BasicLit)
		return ok1 && ok2 && bl1.Lit == bl2.Lit && bl1.Kind == bl2.Kind
	}
}

func (ix *Index) evaluateInternal(ctx Context, setVal any) (any, reflect.Kind, error) {
	if i, ik, err := ix.Index.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else if v, vk, err := ix.X.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else {
		vv := reflect.ValueOf(v)
		switch vk {
		case reflect.Slice:
			if ik != reflect.Int64 {
				return nil, 0, fmt.Errorf("%s: index value is not integer", ix.ErrPos())
			} else if ind := int(i.(int64)); ind < 0 || ind >= vv.Len() {
				return nil, 0, fmt.Errorf("%s: index %d out of range, array length %d", ix.ErrPos(), ind, vv.Len())
			} else if setVal != nil {
				vv.Index(ind).Set(reflect.ValueOf(setVal))
				return nil, 0, nil
			} else {
				vi := vv.Index(ind)
				k := vi.Kind()
				if k == reflect.Interface {
					k = vi.Elem().Kind()
				}
				return vi.Interface(), k, nil
			}
		case reflect.Map:
			key := reflect.ValueOf(i)
			if setVal != nil {
				vv.SetMapIndex(key, reflect.ValueOf(setVal))
				return nil, 0, nil
			} else {
				vi := vv.MapIndex(key)
				k := vi.Kind()
				if k == reflect.Interface {
					k = vi.Elem().Kind()
				}
				if (vi != reflect.Value{}) && vi.CanInterface() {
					return vi.Interface(), k, nil
				} else {
					return nil, 0, fmt.Errorf("map index %v is not exist", i)
				}
			}
		case TransformSlice:
			if setVal != nil {
				panic("cook internal error, transform slice is not handle properly")
			}
			return v.(*iTransform).Transform(ctx, i)
		case TransformMap:
			if setVal != nil {
				panic("cook internal error, transform map is not handle properly")
			}
			return v.(*iTransform).Transform(ctx, i)
		default:
			return nil, 0, fmt.Errorf("index on unsupport type %s", vk)
		}
	}
}

// SubValue Evaluate return sub string or a slice of original slice
func (sv *SubValue) Evaluate(ctx Context) (any, reflect.Kind, error) {
	if si, _, err := sv.Range.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else if v, vk, err := sv.X.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else if sii, ok := si.([]int64); !ok {
		return nil, 0, fmt.Errorf("invalid range %s an integer range is required", sv.Range)
	} else {
		vv := reflect.ValueOf(v)
		if sii[0] < 0 || int(sii[1]) > vv.Len() {
			return nil, 0, fmt.Errorf("slice index %d..%d out of range, length %d", sii[0], sii[1], vv.Len())
		} else if sii[0] == 0 && int(sii[1]) == vv.Len() {
			// no sub string or slice of slice need in this case.
			return v, vk, nil
		}
		switch vk {
		case reflect.String:
			return v.(string)[sii[0]:sii[1]], reflect.String, nil
		case reflect.Slice:
			return vv.Slice(int(sii[0]), int(sii[1])).Interface(), vk, nil
		default:
			return nil, 0, fmt.Errorf("slice of type %s is not supported", vk)
		}
	}
}

// Range Evaluate return a result of slice of 2 items value if not error occurred during evaluation
func (r *Interval) Evaluate(ctx Context) (any, reflect.Kind, error) {
	step, stk := any(int64(1)), reflect.Int64
	if a, ak, err := r.A.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else if b, bk, err := r.B.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else {
		if ak == reflect.String {
			if a, ak, err = convertToNum(a.(string)); err != nil {
				return nil, 0, err
			}
		}
		if bk == reflect.String {
			if b, bk, err = convertToNum(b.(string)); err != nil {
				return nil, 0, err
			}
		}
		if r.Step != nil {
			if step, stk, err = r.Step.Evaluate(ctx); err != nil {
				return nil, 0, err
			} else if stk != reflect.Int64 && stk != reflect.Float64 {
				return nil, 0, fmt.Errorf("step value %v must be greater than 0", step)
			}
		}
		if (ak != reflect.Float64 && ak != reflect.Int64) || (bk != reflect.Float64 && bk != reflect.Int64) {
			return nil, 0, fmt.Errorf("interval required integer or float value")
		} else if ak == reflect.Float64 || bk == reflect.Float64 {
			// we know that a and b is either int64 or float64, ignore the error
			fa, _ := convertToFloat(ctx, a, ak)
			fb, _ := convertToFloat(ctx, b, bk)
			st, _ := convertToFloat(ctx, step, stk)
			// return empty slice if range is not possible for looping
			// ex: ]1..A] where A value is dynamic, if A is 1 then there is nothing to execute or evaluate
			if (!r.AInclude || !r.BInclude) && fa == fb {
				return []float64{}, reflect.Slice, nil
			}
			if !r.AInclude {
				if fa > fb {
					fa -= st
				} else {
					fa += st
				}
			}
			if !r.BInclude {
				if fa > fb {
					fb += st
				} else {
					fb -= st
				}
			}
			return []float64{fa, fb, st}, reflect.Slice, nil
		} else {
			ia, ib := a.(int64), b.(int64)
			st, _ := convertToInt(ctx, step, stk)
			// return empty slice if range is not possible for looping
			// ex: ]1..A] where A value is dynamic, if A is 1 then there is nothing to execute or evaluate
			if (!r.AInclude || !r.BInclude) && ia == ib {
				return []int64{}, reflect.Slice, nil
			}
			if !r.AInclude {
				if ia > ib {
					ia -= st
				} else {
					ia += st
				}
			}
			if !r.BInclude {
				if ia > ib {
					ib += st
				} else {
					ib -= st
				}
			}
			return []int64{ia, ib, st}, reflect.Slice, nil
		}
	}
}

// OSysCheck Evaluate return true if current operating system is match against expression OS value
func (osc *OSysCheck) Evaluate(ctx Context) (any, reflect.Kind, error) {
	return osc.OS.String() == runtime.GOOS, reflect.Bool, nil
}

// Exists Evaluate verify whether a variable, file/folder, command or function is existed
func (e *Exists) Evaluate(ctx Context) (any, reflect.Kind, error) {
	if e.Op == token.FD {
		fp, kind, err := e.X.Evaluate(ctx)
		if err != nil {
			return nil, 0, err
		} else if kind != reflect.String {
			return nil, 0, fmt.Errorf("value %v cannot represent file path", fp)
		} else {
			_, err = os.Stat(fp.(string))
			return err == nil, reflect.Bool, nil
		}
	} else if ident, ok := e.X.(*Ident); ok {
		switch e.Op {
		case token.AT:
			return function.GetFunction(ident.Name) != nil, reflect.Bool, nil
		case token.HASH:
			_, err := exec.LookPath(ident.Name)
			return err == nil, reflect.Bool, nil
		default:
			v, _, _ := ctx.GetVariable(ident.Name)
			return v != nil, reflect.Bool, nil
		}
	} else if ix, ok := e.X.(*Index); ok {
		v, _, err := ix.Evaluate(ctx)
		return err == nil && v != nil, reflect.Bool, nil
	} else {
		panic(fmt.Sprintf("cook internal error: illegal token %s use with exists expression", e.Op))
	}
}

// Call Evaluate execute one of the following type an external command line, a target or a function
func (c *Call) Evaluate(ctx Context) (any, reflect.Kind, error) {
	if c.FuncLit != nil {
		return c.FuncLit.Execute(ctx, c.Args)
	}

	switch c.Kind {
	case token.HASH:
		if args, err := c.args(ctx); err != nil {
			return nil, 0, err
		} else {
			cmd := exec.Command(c.Name, args...)
			dir, err := os.Getwd()
			if err != nil {
				return nil, 0, err
			}
			cmd.Dir = dir
			if c.pipeCmdArgs != "" {
				if w, err := cmd.StdinPipe(); err != nil {
					return nil, 0, err
				} else if _, err = w.Write([]byte(c.pipeCmdArgs)); err != nil {
					w.Close()
					return nil, 0, err
				} else if err = w.Close(); err != nil {
					return nil, 0, err
				}
			} else {
				cmd.Stdin = os.Stdin
			}
			if !c.OutputResult {
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err = cmd.Run(); err != nil {
					return nil, 0, err
				} else {
					return "", reflect.String, nil
				}
			} else {
				result, err := cmd.Output()
				if err != nil {
					return nil, 0, err
				} else {
					return string(result), reflect.String, nil
				}
			}
		}
	case token.AT:
		// priority target, built-in command, developer defined function
		t := ctx.GetTarget(c.Name)
		if t != nil {
			if args, err := c.funcArgs(ctx); err != nil {
				return nil, 0, err
			} else {
				t.Execute(ctx, args)
				return nil, 0, nil
			}
		}
		// command
		f := ctx.GetCommand(c.Name)
		if f != nil {
			if args, err := c.funcArgs(ctx); err != nil {
				return nil, 0, err
			} else {
				if v, err := f.Apply(args); err != nil {
					return nil, 0, fmt.Errorf("%s: %w", c.ErrPos(), err)
				} else {
					return v, reflect.ValueOf(v).Kind(), nil
				}
			}
		}
		// function
		fn := ctx.GetFunction(c.Name)
		if fn == nil {
			return nil, 0, fmt.Errorf("target or function %s is not exist", c.Name)
		} else {
			return fn.Execute(ctx, c.Args)
		}
	}
	// parser should ensure it
	panic(fmt.Sprintf("invalid call expression %s", c.Kind))
}

func (c *Call) args(ctx Context) ([]string, error) {
	args := make([]string, 0, len(c.Args))
	for _, arg := range c.Args {
		if v, vk, err := arg.Evaluate(ctx); err != nil {
			return nil, err
		} else {
			switch vk {
			case reflect.Array, reflect.Slice:
				if args, err = expandArrayTo(ctx, reflect.ValueOf(v), args); err != nil {
					return nil, err
				}
			default:
				if v, err = convertToString(ctx, v, vk); err != nil {
					return nil, fmt.Errorf("%s: %w", c.Base.ErrPos(), err)
				}
				args = append(args, v.(string))
			}
		}
	}
	return args, nil
}

func (c *Call) funcArgs(ctx Context) ([]*args.FunctionArg, error) {
	sargs := make([]*args.FunctionArg, 0, len(c.Args))
	for _, arg := range c.Args {
		if v, vk, err := arg.Evaluate(ctx); err != nil {
			return nil, err
		} else {
			switch vk {
			case reflect.Array, reflect.Slice:
				sargs = expandArrayToFuncArgs(ctx, reflect.ValueOf(v), sargs)
			default:
				sargs = append(sargs, &args.FunctionArg{Val: v, Kind: vk})
			}
		}
	}
	if c.pipeBuiltInArgs != nil {
		sargs = append(sargs, c.pipeBuiltInArgs)
	}
	return sargs, nil
}

func (c *Call) setPipeArgument(ctx Context, v any, k reflect.Kind) (err error) {
	if c.Kind == token.HASH {
		if c.pipeCmdArgs, err = convertToString(ctx, v, k); err != nil {
			return err
		}
	} else {
		c.pipeBuiltInArgs = &args.FunctionArg{Val: v, Kind: k}
	}
	return nil
}

func (pp *Pipe) Evaluate(ctx Context) (any, reflect.Kind, error) {
	pp.X.OutputResult = true
	if result, kind, err := pp.X.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else if pp.Y != nil {
		if c, ok := pp.Y.(*Call); ok {
			if err = c.setPipeArgument(ctx, result, kind); err != nil {
				return nil, 0, err
			}
			c.OutputResult = true
			return c.Evaluate(ctx)
		} else if p, ok := pp.Y.(*Pipe); ok {
			if err = p.X.setPipeArgument(ctx, result, kind); err != nil {
				return nil, 0, err
			}
			return p.Evaluate(ctx)
		} else if rd, ok := pp.Y.(*RedirectTo); ok {
			if c, ok := rd.Caller.(*Call); ok {
				if err = c.setPipeArgument(ctx, result, kind); err != nil {
					return nil, 0, err
				}
				c.OutputResult = true
				return rd.Evaluate(ctx)
			}
		}
		panic("cook internal error: Pipe support only call, redirect and pipe expression itself")
	} else {
		return result, kind, nil
	}
}

// ReadFrom Evaluate return content of a file
func (rf *ReadFrom) Evaluate(ctx Context) (any, reflect.Kind, error) {
	if v, k, err := rf.File.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else if k != reflect.String {
		return nil, 0, fmt.Errorf("readfrom expression required string")
	} else if b, err := os.ReadFile(v.(string)); err != nil {
		return nil, 0, err
	} else {
		return string(b), reflect.String, nil
	}
}

// WriteTo Evaluate write/append the data to the file
func (rt *RedirectTo) Evaluate(ctx Context) (any, reflect.Kind, error) {
	files, err := stringOf(ctx, rt.Files...)
	if err != nil {
		return nil, 0, err
	}

	if call, ok := rt.Caller.(*Call); ok {
		call.OutputResult = true
	}
	v, vk, err := rt.Caller.Evaluate(ctx)
	if err != nil {
		return nil, 0, err
	}

	var b []byte
	var ok bool
	if vk == reflect.String {
		b = []byte(v.(string))
	} else if b, ok = v.([]byte); !ok {
		goto tryReader
	}
	// write all b to each file
	for _, f := range files {
		_, err := os.Stat(f)
		if rt.Append && !os.IsNotExist(err) {
			if fs, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0700); err != nil {
				return nil, 0, err
			} else if _, err = fs.Write(b); err != nil {
				fs.Close()
				return nil, 0, err
			}
		} else if err = os.WriteFile(f, b, 0700); err != nil {
			return nil, 0, err
		}
	}
	return nil, 0, nil

tryReader:
	var reader io.Reader
	if r, ok := v.(io.ReadCloser); ok {
		defer r.Close()
		reader = r
	} else if r, ok := v.(io.Reader); ok {
		reader = r
	} else {
		return nil, 0, fmt.Errorf("write to file unsupport type %s", vk)
	}
	var writer []io.Writer
	for _, f := range files {
		flags := os.O_WRONLY | os.O_CREATE
		if rt.Append {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		w, err := os.OpenFile(f, flags, 0700)
		if err != nil {
			return nil, 0, err
		}
		defer w.Close()
		writer = append(writer, w)
	}
	_, err = io.Copy(io.MultiWriter(writer...), reader)
	return nil, 0, err
}

// Paran Evaluate execute inner node and return it's response
func (p *Paren) Evaluate(ctx Context) (any, reflect.Kind, error) {
	return p.Inner.Evaluate(ctx)
}

func (un *Unary) Evaluate(ctx Context) (any, reflect.Kind, error) {
	v, vk, err := un.X.Evaluate(ctx)
	if err != nil {
		return nil, 0, err
	}
	switch {
	case un.Op == token.ADD:
		if vk == reflect.String {
			if v, vk, err = convertToNum(v.(string)); err != nil {
				return nil, 0, err
			}
		}
		if vk == reflect.Float64 || vk == reflect.Int64 {
			return v, vk, nil
		}
	case un.Op == token.SUB:
		if vk == reflect.String {
			if v, vk, err = convertToNum(v.(string)); err != nil {
				return nil, 0, err
			}
		}
		if vk == reflect.Float64 {
			return -v.(float64), vk, nil
		} else if vk == reflect.Int64 {
			return -v.(int64), vk, nil
		}
	case un.Op == token.XOR && vk == reflect.Int64:
		return ^v.(int64), vk, nil
	case un.Op == token.NOT:
		switch vk {
		case reflect.Float64:
			return v.(float64) != 0.0, reflect.Bool, nil
		case reflect.Int64:
			return v.(int64) != 0, reflect.Bool, nil
		case reflect.Bool:
			return !v.(bool), vk, nil
		case reflect.String:
			return v.(string) != "", reflect.Bool, nil
		case reflect.Array:
			return len(v.([]any)) > 0, reflect.Bool, nil
		case reflect.Map:
			return len(v.([]any)) > 0, reflect.Bool, nil
		default:
			return v != nil, reflect.Bool, nil
		}
	case un.Op == token.FD:
		// sizeof expression must check if node is unary and it's indicate file type Operator
		return v.(string), reflect.String, nil
	}
	return nil, reflect.Invalid, fmt.Errorf("unary operator %s is not supported on value %v`", un.Op, v)
}

// IncDecExpr Evaluate increase or delete value by 1
func (idc *IncDec) Evaluate(ctx Context) (v any, k reflect.Kind, err error) {
	// handle variable
	defer func() {
		if ident, ok := idc.X.(*Ident); ok && err == nil {
			ctx.SetVariable(ident.Name, v, k, nil)
		}
	}()
	v, k, err = idc.X.Evaluate(ctx)
	if err != nil {
		return nil, 0, err
	}
	// step value for increase or decrease
	step := int64(0)
	switch idc.Op {
	case token.INC:
		step = 1
	case token.DEC:
		step = -1
	default:
		panic("illegal state parser, operator must be increment or decrement")
	}
retryOnString:
	switch k {
	case reflect.Float64:
		return v.(float64) + float64(step), k, nil
	case reflect.Int64:
		return v.(int64) + step, k, nil
	case reflect.String:
		if v, k, err = convertToNum(v.(string)); err != nil {
			return nil, 0, err
		} else if k == reflect.Int64 || k == reflect.Float64 {
			goto retryOnString
		}
	}
	return nil, 0, fmt.Errorf("unsupported operator %s on %s of kind %s", idc.Op, idc.X, k)
}

// Binary Evaluate return result of pair operation
func (b *Binary) Evaluate(ctx Context) (any, reflect.Kind, error) {
	if vl, vkl, err := b.L.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else if vr, vkr, err := b.R.Evaluate(ctx); err != nil {
		return nil, 0, err
	} else {
		switch {
		case b.Op == token.ADD:
			return addOperator(ctx, vl, vr, vkl, vkr)
		case token.ADD < b.Op && b.Op < token.LAND:
			return numOperator(ctx, b.Op, vl, vr, vkl, vkr)
		case token.EQL <= b.Op && b.Op <= token.GEQ:
			return logicOperator(ctx, b.Op, vl, vr, vkl, vkr)
		case vkl == vkr && vkl == reflect.Bool:
			if b.Op == token.LAND {
				return vl.(bool) && vr.(bool), reflect.Bool, nil
			} else if b.Op == token.LOR {
				return vl.(bool) || vr.(bool), reflect.Bool, nil
			}
		}
		return nil, 0, fmt.Errorf("unsupported operator %s on value %v and %v", b.Op, vl, vr)
	}
}

// Transformation Evaluate apply tranform on an array or map
func (t *Transformation) Evaluate(ctx Context) (any, reflect.Kind, error) {
	tv, k, err := t.Ident.Evaluate(ctx)
	if err != nil {
		return nil, 0, err
	}
	// if equal then we should update value in place or self update otherwise
	// store transform meta instead
	if t.to.IsEqual(t.Ident) {
		// self update
		switch k {
		case reflect.Slice:
			vv := reflect.ValueOf(tv)
			for i := 0; i < vv.Len(); i++ {
				indv := vv.Index(i)
				v, _, err := t.Fn.internalExecute(ctx, 2, func(vi int) (any, reflect.Kind, error) {
					if vi == 0 {
						return int64(i), reflect.Int64, nil
					} else {
						return indv.Interface(), indv.Kind(), nil
					}
				})
				if err != nil {
					return nil, 0, nil
				} else {
					indv.Set(reflect.ValueOf(v))
				}
			}
		case reflect.Map:
			vv := reflect.ValueOf(tv)
			keys := vv.MapKeys()
			for _, key := range keys {
				indv := vv.MapIndex(key)
				v, _, err := t.Fn.internalExecute(ctx, 2, func(vi int) (any, reflect.Kind, error) {
					if vi == 0 {
						return key.Interface(), key.Kind(), nil
					} else {
						return indv.Interface(), indv.Kind(), nil
					}
				})
				if err != nil {
					return nil, 0, nil
				} else {
					vv.SetMapIndex(key, reflect.ValueOf(v))
				}
			}
		default:
			return nil, 0, fmt.Errorf("transform can only apply on array or map variable")
		}
	} else {
		// apply transform via function literal
		var it *iTransform
		var tk reflect.Kind
		switch k {
		case reflect.Slice:
			vv := reflect.ValueOf(tv)
			tk = TransformSlice
			it = &iTransform{
				Len: func() int { return vv.Len() },
				Source: func(ctx Context, i any) (any, reflect.Kind, error) {
					v := vv.Index(int(i.(int64)))
					if v.Kind() == reflect.Interface {
						return v.Interface(), v.Elem().Kind(), nil
					}
					return v.Interface(), v.Kind(), nil
				},
				Value: func(ctx Context, i, val any) (any, reflect.Kind, error) {
					return t.Fn.internalExecute(ctx, 2, func(iv int) (any, reflect.Kind, error) {
						if iv == 0 {
							return i.(int64), reflect.Int64, nil
						} else {
							return val, reflect.ValueOf(val).Kind(), nil
						}
					})
				},
			}
		case reflect.Map:
			vv := reflect.ValueOf(tv)
			tk = TransformMap
			it = &iTransform{
				Len: func() int { return vv.Len() },
				Source: func(ctx Context, i any) (any, reflect.Kind, error) {
					v := vv.MapIndex(reflect.ValueOf(i))
					if !v.IsValid() {
						return nil, 0, fmt.Errorf("index %v is not exist", i)
					}
					return v.Interface(), v.Kind(), nil
				},
				Value: func(ctx Context, i, val any) (any, reflect.Kind, error) {
					return t.Fn.internalExecute(ctx, 2, func(iv int) (any, reflect.Kind, error) {
						if iv == 0 {
							return i, reflect.ValueOf(i).Kind(), nil
						} else {
							return val, reflect.ValueOf(val).Kind(), nil
						}
					})
				},
			}
		case TransformSlice:
			ts := tv.(*iTransform)
			tk = TransformSlice
			it = &iTransform{
				Len:    func() int { return ts.Len() },
				Source: ts.Transform,
				Value: func(ctx Context, i, val any) (any, reflect.Kind, error) {
					return t.Fn.internalExecute(ctx, 2, func(iv int) (any, reflect.Kind, error) {
						if iv == 0 {
							return i.(int64), reflect.Int64, nil
						} else {
							return val, reflect.ValueOf(val).Kind(), nil
						}
					})
				},
			}
		case TransformMap:
			ts := tv.(*iTransform)
			tk = TransformMap
			it = &iTransform{
				Len:    func() int { return ts.Len() },
				Source: ts.Transform,
				Value: func(ctx Context, i, val any) (any, reflect.Kind, error) {
					return t.Fn.internalExecute(ctx, 2, func(iv int) (any, reflect.Kind, error) {
						if iv == 0 {
							return i, reflect.ValueOf(i).Kind(), nil
						} else {
							return val, reflect.ValueOf(val).Kind(), nil
						}
					})
				},
			}
		default:
			return nil, 0, fmt.Errorf("transform can only apply on array or map variable")
		}
		t.to.Set(ctx, it, tk, nil)
	}
	return nil, 0, nil
}
