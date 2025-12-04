package function

import (
	"fmt"
	"strconv"

	"github.com/cozees/cook/pkg/runtime/args"
)

type FuncHandler func(f Function, i any) (any, error)

type Function interface {
	Apply([]*args.FunctionArg) (any, error)
	Name() string
	Flags() *args.Flags
	Alias() []string
}

// store function reference by name
var funcStore map[string]Function = make(map[string]Function)

func IsExist(name string) bool         { return funcStore[name] != nil }
func GetFunction(name string) Function { return funcStore[name] }

func registerFunction(f Function) {
	funcStore[f.Name()] = f
	if alias := f.Alias(); alias != nil {
		for _, a := range alias {
			funcStore[a] = f
		}
	}
}

type BaseFunction struct {
	fnFlags   *args.Flags
	nameAlias []string
	handler   FuncHandler
}

func NewBaseFunction(flags *args.Flags, fh FuncHandler, alias ...string) *BaseFunction {
	flags.Aliases = alias
	return &BaseFunction{
		fnFlags:   flags,
		nameAlias: alias,
		handler:   fh,
	}
}

func (bf *BaseFunction) Name() string       { return bf.fnFlags.FuncName }
func (bf *BaseFunction) Alias() []string    { return bf.nameAlias }
func (bf *BaseFunction) Flags() *args.Flags { return bf.fnFlags }

func (bf *BaseFunction) Apply(args []*args.FunctionArg) (any, error) {
	i, err := bf.fnFlags.ParseFunctionArgs(args)
	if bf.handler != nil && i != nil {
		i, err = bf.handler(bf, i)
	}
	return i, err
}

func toString(i any) (string, error) {
	switch v := i.(type) {
	case string:
		return v, nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return "", fmt.Errorf("value %v cannot convert to string", i)
	}
}
