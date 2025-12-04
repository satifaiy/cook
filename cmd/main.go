package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/cozees/cook/pkg/cook/parser"
	"github.com/cozees/cook/pkg/runtime/args"
	"github.com/cozees/cook/pkg/runtime/function"
)

func main() {
	opts, err := args.ParseMainArgument(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	} else if opts.IsHelp {
		PrintHelp(opts.FuncMeta)
		os.Exit(0)
	} else if opts.FuncMeta != nil {
		executeFunction(opts)
		os.Exit(0)
	}

	p := parser.NewParser()
	cook, err := p.Parse(opts.Cookfile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	} else if len(opts.Targets) > 0 {
		err = cook.ExecuteWithTarget(opts.Args, opts.Targets...)
	} else {
		err = cook.Execute(opts.Args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
}

func executeFunction(opts *args.MainOptions) {
	fn := function.GetFunction(opts.FuncMeta.Name)
	if fn == nil {
		fmt.Fprintln(os.Stderr, "function", opts.FuncMeta.Name, "is not exist")
		os.Exit(1)
	}
	result, err := fn.Apply(opts.FuncMeta.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error while execute function @%s: %s\n", opts.FuncMeta.Name, err.Error())
		os.Exit(1)
	} else if result == nil {
		fmt.Fprintf(os.Stdout, "\n")
		os.Exit(0)
	}
	// build output
	var output func(w *bufio.Writer, v reflect.Value) error
	output = func(w *bufio.Writer, v reflect.Value) (err error) {
		vk := v.Kind()
	revisit:
		switch {
		case vk == reflect.Interface:
			v = v.Elem()
			vk = v.Kind()
			goto revisit
		case vk <= reflect.Complex128 || vk == reflect.String:
			if _, err = fmt.Fprintf(w, "%v", v.Interface()); err != nil {
				return err
			}
		case vk == reflect.Array || vk == reflect.Slice:
			if err = w.WriteByte('['); err != nil {
				return err
			}
			size := v.Len()
			for i := range size {
				sv := v.Index(i)
				if err = output(w, sv); err != nil {
					return err
				} else if i+1 < size {
					w.WriteString(", ")
				}
			}
			if err = w.WriteByte(']'); err != nil {
				return err
			}
		case vk == reflect.Map:
			if err = w.WriteByte('{'); err != nil {
				return err
			}
			keys := v.MapRange()
			for hasItem := keys.Next(); hasItem; {
				kv := keys.Key()
				sv := v.MapIndex(kv)
				// move next right away so we can check later one that to add comma
				hasItem = keys.Next()
				if err = output(w, kv); err != nil {
					return err
				} else if _, err = w.WriteString(": "); err != nil {
					return err
				} else if err = output(w, sv); err != nil {
					return err
				} else if hasItem {
					w.WriteString(", ")
				}
			}
			if err = w.WriteByte('}'); err != nil {
				return err
			}
		default:
			if v.CanInterface() {
				iv := v.Interface()
				var reader io.Reader
				if r, ok := iv.(io.ReadCloser); ok {
					defer r.Close()
					reader = r
				} else if r, ok := iv.(io.Reader); ok {
					reader = r
				}
				if reader != nil {
					if _, err = io.Copy(w, reader); err != nil {
						return err
					}
				}
			} else {
				return fmt.Errorf("function return unsupported kind %s", vk)
			}
		}
		return nil
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	if err = output(w, reflect.ValueOf(result)); err != nil {
		fmt.Fprintf(os.Stderr, "error while writing function @%s output: %s\n", opts.FuncMeta.Name, err.Error())
		os.Exit(1)
	}
	w.WriteByte('\n')
}
