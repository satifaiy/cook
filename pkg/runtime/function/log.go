package function

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"github.com/cozees/cook/pkg/runtime/args"
)

func AllLogFlags() []*args.Flags {
	return []*args.Flags{printFlags}
}

type printOption struct {
	Strip  bool `flag:"strip"`  // strip whitespace before print for each argument
	OmitNL bool `flag:"omitln"` // add newline, default yes
	Echo   bool `flag:"echo"`
	Args   []any
}

const (
	printDesc = `The print function write the arguments as the string into standard output if flag "echo"
				 is not given otherwise the string result is return from the function instead.`
	echoDesc   = `Tell print function to return the result instead of writing the result in standard output.`
	omitnlDesc = `Tell print function to not add a newline at the end of the result.`
	stripDesc  = `Tell print function to remove all leading and trailing whitespace from each given argument.`
)

var printFlags = &args.Flags{
	Flags: []*args.Flag{
		{Short: "e", Long: "echo", Description: echoDesc},
		{Short: "n", Long: "omitln", Description: omitnlDesc},
		{Short: "s", Long: "strip", Description: stripDesc},
	},
	Result:      reflect.TypeOf((*printOption)(nil)).Elem(),
	FuncName:    "print",
	Example:     "@print -e text",
	ShortDesc:   "print arguments to the standard output or return them as a single string.",
	Usage:       "@print [-ens] ARG [ARG ...]",
	Description: printDesc,
}

var printFn = NewBaseFunction(printFlags, func(bf Function, i any) (v any, err error) {
	opts := i.(*printOption)
	txt := ""
	if len(opts.Args) > 0 {
		buf := bytes.NewBufferString("")
		s := ""
		for i := range opts.Args {
			if s, err = toString(opts.Args[i]); err != nil {
				return nil, err
			}
			if opts.Strip {
				buf.WriteByte(' ')
				buf.WriteString(strings.TrimSpace(s))
			} else {
				buf.WriteByte(' ')
				buf.WriteString(s)
			}
		}
		txt = buf.String()[1:]
	}
	if opts.OmitNL {
		if opts.Echo {
			return txt, nil
		}
		fmt.Print(txt)
	} else {
		if opts.Echo {
			return txt + "\n", nil
		}
		fmt.Println(txt)
	}
	return nil, nil
})

func init() {
	registerFunction(printFn)
}
