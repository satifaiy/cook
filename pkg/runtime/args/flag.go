package args

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
)

var (
	ErrFlagSyntax  = fmt.Errorf("invalid flag format. E.g. --name:s, --name:i:f")
	ErrAllowFormat = fmt.Errorf("only i, f, s, b, a format is allowed")
	ErrVarSyntax   = fmt.Errorf("variable flag must start with --")
)

const (
	defaultCookfile = "Cookfile"
)

type Redirect uint8

type FunctionMeta struct {
	Name     string
	Redirect string
	Files    []string
	Args     []*FunctionArg
}

type MainOptions struct {
	Cookfile string
	Targets  []string
	Args     map[string]any
	FuncMeta *FunctionMeta
	IsHelp   bool
}

func ParseMainArgument(args []string) (*MainOptions, error) {
	mo := &MainOptions{Cookfile: defaultCookfile}
	// check if first argument start with @ which is refer to a function name.
	// cook will execute the function directly. The second argument onward will be
	// use as function argument
	if len(args) >= 1 && strings.HasPrefix(args[0], "@") {
		mo.FuncMeta = &FunctionMeta{Name: args[0][1:]}
		readFrom := ""
		for _, arg := range args[1:] {
			if readFrom != "" {
				b, err := os.ReadFile(arg)
				if err != nil {
					return nil, err
				}
				mo.FuncMeta.Args = append(mo.FuncMeta.Args, &FunctionArg{
					Val:  string(b),
					Kind: reflect.String,
				})
				readFrom = ""
				continue
			}
			if mo.FuncMeta.Redirect != "" {
				mo.FuncMeta.Files = append(mo.FuncMeta.Files, arg)
				continue
			}
			// on unix platform or any platform that support redirect syntax
			// the os or shell program that execute Cook will consume direct path
			// and leave no redirect info here
			switch arg {
			case "<":
				readFrom = arg
			case ">", ">>":
				mo.FuncMeta.Redirect = arg
			default:
				mo.FuncMeta.Args = append(mo.FuncMeta.Args, &FunctionArg{
					Val:  arg,
					Kind: reflect.String,
				})
			}
		}
		return mo, nil
	}

	// handle help
	if len(args) >= 1 && args[0] == "help" {
		mo.IsHelp = true
		if len(args) == 2 && strings.HasPrefix(args[1], "@") {
			mo.FuncMeta = &FunctionMeta{Name: args[1][1:]}
		}
		return mo, nil
	}

	// parse normal argument
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--"):
			val := ""
			ieql := strings.IndexByte(arg, '=')
			if ieql == -1 {
				ieql = len(arg)
				if n := i + 1; n < len(args) {
					i = n
					val = args[i]
				}
			} else {
				val = arg[ieql+1:]
			}
			vname, p, s, err := parseFlagFormat(arg[2:ieql])
			if err != nil {
				return nil, err
			}
			// create args if first encouter
			if mo.Args == nil {
				mo.Args = make(map[string]any)
			}
			// special case for map
			var pv, pk any
			if s != reflect.Invalid {
				icolon := strings.IndexByte(val, ':')
				if icolon < 1 {
					return nil, fmt.Errorf("invalid flag value map %s, must be format of key:value", val)
				}
				if pk, err = parseFlagValue(p, val[:icolon]); err != nil {
					return nil, err
				} else if pv, err = parseFlagValue(s, val[icolon+1:]); err != nil {
					return nil, err
				} else if mp, ok := mo.Args[vname]; !ok {
					mo.Args[vname] = map[any]any{pk: pv}
				} else if mpv, ok := mp.(map[any]any); ok {
					mpv[pk] = pv
				} else {
					return nil, fmt.Errorf("variable %s value %v (%s) is not a map", vname, mp, reflect.ValueOf(mp).Kind())
				}
			} else if pv, err = parseFlagValue(p, val); err != nil {
				return nil, err
			} else if exist, ok := mo.Args[vname]; ok {
				vex := reflect.ValueOf(exist)
				if vex.Kind() == reflect.Slice {
					mo.Args[vname] = reflect.Append(vex, reflect.ValueOf(pv)).Interface()
				} else {
					mo.Args[vname] = []any{exist, pv}
				}
			} else {
				mo.Args[vname] = pv
			}
		case strings.HasPrefix(arg, "-"):
			if arg == "-c" {
				if n := i + 1; n < len(args) {
					i = n
					mo.Cookfile = args[i]
				}
				break
			}
			return nil, ErrVarSyntax
		default:
			if len(arg) == 0 || !('a' <= lower(arg[0]) && lower(arg[0]) <= 'z' || arg[0] == '_') {
				return nil, fmt.Errorf("invalid target name %s", arg)
			}
			if mo.Targets == nil {
				mo.Targets = make([]string, 0, 2)
			}
			mo.Targets = append(mo.Targets, arg)
		}
	}
	return mo, nil
}

type Flag struct {
	Short       string // single character, e.g. -e, -e
	Long        string // more 2 character, e.g. --name or -name
	Description string
}

func (flag *Flag) Set(fname string, field reflect.Value, val string, nextArg any, nextArgKind reflect.Kind) (advance bool, err error) {
	// get field type
	t := field.Kind()
	switch {
	case t == reflect.Array:
		return false, fmt.Errorf("flag type must be a slice not an array")
	case t == reflect.Bool:
		if val != "" {
			return false, fmt.Errorf("boolean flag should not have extral value")
		}
		field.Set(reflect.ValueOf(true))
		return false, nil
	case val == "":
		if nextArg == nil {
			return false, fmt.Errorf("not enough argument, missing value for flag %s", flag.Long)
		}
	}
	// special case for map and array
	te := field.Type()
	nextArgVal := reflect.ValueOf(nextArg)
	switch t {
	case reflect.Slice:
		eKind := te.Elem().Kind()
		if nextArg != nil {
			// if it was a slice or same as element of a slice
			if nextArgKind == t {
				sliceEKind := nextArgVal.Type().Elem().Kind()
				if sliceEKind != eKind {
					return false, fmt.Errorf("argment value %v element type %s is not match with %s", nextArg, sliceEKind, eKind)
				}
				advance = true
				field.Set(nextArgVal)
				break
			} else if nextArgKind == eKind || (eKind == reflect.Interface && nextArgKind != reflect.String) {
				field.Set(reflect.Append(field, nextArgVal))
				advance = true
				break
			} else if nextArgKind != reflect.String {
				return false, fmt.Errorf("function argument %v (%s) is not match with %s", nextArg, nextArgKind, t)
			}
			advance = true
			val = nextArg.(string)
		}
		// try to convert string to value type "t"
		if fval, err := parseFlagValue(eKind, val); err != nil {
			return false, err
		} else {
			field.Set(reflect.Append(field, reflect.ValueOf(fval)))
		}
	case reflect.Map:
		if nextArg != nil {
			if nextArgKind == t {
				nt := nextArgVal.Type()
				kkind, vkind := te.Key().Kind(), te.Elem().Kind()
				nKeyKind, nValKind := nt.Key().Kind(), nt.Elem().Kind()
				if (kkind != reflect.Interface && nKeyKind != kkind) || (vkind != reflect.Interface && nValKind != vkind) {
					return false, fmt.Errorf("value %v type (%s:%s) is not compatible with %s:%s", nextArg, nKeyKind, nValKind, kkind, vkind)
				}
				advance = true
				if field.IsNil() {
					field.Set(reflect.MakeMap(te))
				}
				for _, kv := range nextArgVal.MapKeys() {
					field.SetMapIndex(kv, nextArgVal.MapIndex(kv))
				}
				break
			} else if nextArgKind != reflect.String {
				return false, fmt.Errorf("function argument %v (%s) is not a map", nextArg, nextArgKind)
			}
			advance = true
			val = nextArg.(string)
		}

		icolon := strings.IndexByte(val, ':')
		if icolon < 1 {
			return false, fmt.Errorf("invalid map entry %s for flag %s", val, flag.Long)
		}
		var kval, vval any
		if kval, err = parseFlagValue(te.Key().Kind(), val[:icolon]); err != nil {
			return false, err
		}
		// special case for slice in map
		ekind := te.Elem().Kind()
		if ekind == reflect.Slice {
			ekind := te.Elem().Elem().Kind()
			if vval, err = parseFlagValue(ekind, val[icolon+1:]); err != nil {
				return false, err
			}
			key := reflect.ValueOf(kval)
			if field.IsNil() {
				field.Set(reflect.MakeMap(field.Type()))
			}
			slice := field.MapIndex(key)
			if slice == (reflect.Value{}) {
				slice = reflect.MakeSlice(te.Elem(), 0, 1)
			}
			field.SetMapIndex(key, reflect.Append(slice, reflect.ValueOf(vval)))
		} else if vval, err = parseFlagValue(ekind, val[icolon+1:]); err != nil {
			return false, err
		} else {
			if field.IsNil() {
				field.Set(reflect.MakeMap(field.Type()))
			}
			field.SetMapIndex(reflect.ValueOf(kval), reflect.ValueOf(vval))
		}
	default:
		if nextArg != nil {
			if t == nextArgKind {
				field.Set(nextArgVal)
				advance = true
				break
			} else if nextArgKind != reflect.String {
				return false, fmt.Errorf("value '%v' type %s is not compatible with field '%s' type %s", nextArg, nextArgKind, fname, t)
			}
			advance = true
			val = nextArg.(string)
		}
		var vval any
		if vval, err = parseFlagValue(t, val); err != nil {
			return false, err
		}
		field.Set(reflect.ValueOf(vval))
	}
	return advance, nil
}

type FunctionArg struct {
	Val  any
	Kind reflect.Kind
}

type Flags struct {
	FuncName    string
	Aliases     []string
	Flags       []*Flag
	Result      reflect.Type
	Example     string
	Usage       string
	ShortDesc   string
	Description string
}

func (flags *Flags) Help(md bool, topAnchor string) string {
	return flags.generateUsage(md, topAnchor, nil).String()
}
func (flags *Flags) HelpAsReader(md bool, topAnchor string) io.Reader {
	return flags.generateUsage(md, topAnchor, nil)
}

func (Flags *Flags) HelpFlagVisitor(md bool, topAnchor string, fn func(fw FlagWriter)) io.Reader {
	return Flags.generateUsage(md, topAnchor, fn)
}

func (flags *Flags) generateUsage(md bool, topAnchor string, fn func(fw FlagWriter)) Builder {
	var builder Builder
	if md {
		builder = NewMarkdownBuilder()
	} else {
		builder = NewConsoleBuilder()
	}
	builder.Name(flags.FuncName, flags.ShortDesc, flags.Aliases...)
	builder.Usage(flags.Usage)
	builder.Description(flags.Description)
	if fn != nil {
		builder.FlagVisitor(fn)
	} else {
		builder.Flag(flags.Flags, flags.Result)
	}
	builder.Example(flags.Example, topAnchor)
	return builder
}

func (flags *Flags) Validate() error {
	m := make(map[string]bool)
	for _, flag := range flags.Flags {
		if flag.Short != "" {
			if len(flag.Short) != 1 {
				return fmt.Errorf("short flag %s must have only 1 characters", flag.Short)
			}
			if _, ok := m[flag.Short]; ok {
				return fmt.Errorf("short flag %s already registered", flag.Short)
			}
			m[flag.Short] = true
		}
		if len(flag.Long) < 2 {
			return fmt.Errorf("long flag is required")
		}
		if _, ok := m[flag.Long]; ok {
			return fmt.Errorf("long flag %s already registered", flag.Long)
		}
		m[flag.Long] = true
	}
	return nil
}

func (flags *Flags) ensureStruct() error {
	if kind := flags.Result.Kind(); kind != reflect.Struct {
		return fmt.Errorf("option result must be a struct, giving %s", kind)
	}
	return nil
}

func (flags *Flags) checkFlag(arg string) (flag *Flag, fval string, err error) {
	switch {
	case strings.HasPrefix(arg, "--"):
		if flag, fval, err = flags.findFlag(arg[2:], false); err != nil {
			return
		}
	case strings.HasPrefix(arg, "-"):
		if len(arg) > 2 {
			err = fmt.Errorf("long flag %s required (--)", arg)
			return
		}
		if flag, fval, err = flags.findFlag(arg[1:], true); err != nil {
			return
		}
	}
	return
}

// ParseFlagFunction return a pointer to a struct result if no error occurred.
// Unlike Parse which accept slice of string, `ParseFlagFunction` accept a slice
// of interface value (any value), in order to avoid parsing back and forth between
// string and numeric value. When args values is all strings then `ParseFlagFunction`
// is behave exactly as `Parse` method
func (flags *Flags) ParseFunctionArgs(args []*FunctionArg) (any, error) {
	return flags.parseInternal(nil, args)
}

func (flags *Flags) Parse(args []string) (any, error) {
	return flags.parseInternal(args, nil)
}

func (flags *Flags) parseInternal(args []string, fnArgs []*FunctionArg) (v any, err error) {
	if err = flags.ensureStruct(); err != nil {
		return nil, err
	}
	var (
		flag   *Flag
		fval   string
		val    = reflect.New(flags.Result).Elem()
		length = len(args)
		arg    any
		sarg   string
		narg   any
		nargk  reflect.Kind
		field  reflect.Value
		fname  string // field name
	)
	// set default value to each field if defined
	if err = setDefaultFieldValue(flags.Result, val); err != nil {
		return nil, err
	}
	// check Args arguments
	argsField := val.FieldByName("Args")
	if !argsField.CanSet() || argsField.Kind() != reflect.Slice {
		err = fmt.Errorf("options %s must defined field Args type slice", flags.Result.Name())
		return
	}
	argsKind := argsField.Type().Elem().Kind()

	if length == 0 {
		length = len(fnArgs)
	}
	for i := 0; i < length; i++ {
		flag = nil
		if args != nil {
			arg, sarg = args[i], args[i]
		} else {
			arg, sarg = fnArgs[i].Val, ""
			if fnArgs[i].Kind == reflect.String {
				sarg = arg.(string)
			}
		}
		if flag, fval, err = flags.checkFlag(sarg); err != nil {
			return
		} else if flag == nil {
			if args != nil {
				if arg, err = parseFlagValue(argsKind, sarg); err != nil {
					return nil, err
				}
			} else if nargk = reflect.ValueOf(arg).Kind(); nargk != argsKind && argsKind != reflect.Interface {
				return nil, fmt.Errorf("wrong argument %v type %s required type %s", arg, nargk, argsKind)
			}
			if (argsField == reflect.Value{}) {
				t := reflect.TypeOf(flags.Result)
				argsField = reflect.MakeSlice(t.Elem(), 0, length)
			}
			argsField.Set(reflect.Append(argsField, reflect.ValueOf(arg)))
			continue
		}
		// find field in struct with tag that belong to the flag
		if field, fname, err = findField(flags.Result, val, flag.Long); err != nil {
			return
		} else if !field.CanSet() {
			err = fmt.Errorf("field was not found or not exported for flag %s", flag.Long)
			return
		}
		n := i + 1
		if fval == "" && n < length {
			if args != nil {
				narg = args[n]
				nargk = reflect.String
			} else {
				narg = fnArgs[n].Val
				nargk = fnArgs[n].Kind
			}
		}
		if inc, err := flag.Set(fname, field, fval, narg, nargk); err != nil {
			return nil, err
		} else if inc {
			i = n
		}
	}
	v = val.Addr().Interface()
	return
}

func (flags *Flags) findFlag(s string, short bool) (*Flag, string, error) {
	var val string
	if i := strings.IndexByte(s, '='); i == 0 {
		return nil, "", fmt.Errorf("flag must not start with =")
	} else if i > 0 {
		s, val = s[:i], s[i+1:]
	}
	for _, flag := range flags.Flags {
		if (short && flag.Short == s) || (!short && flag.Long == s) {
			return flag, val, nil
		}
	}
	return nil, "", fmt.Errorf("unrecognize flag %s", s)
}

func parseFlagValue(kind reflect.Kind, v string) (any, error) {
	switch kind {
	case reflect.Int64:
		return strconv.ParseInt(v, 10, 64)
	case reflect.Float64:
		return strconv.ParseFloat(v, 64)
	case reflect.Bool:
		return strconv.ParseBool(v)
	case reflect.String: // s or default treat as string
		return v, nil
	case reflect.Interface:
		if i, err := parseFlagValue(reflect.Int64, v); err == nil {
			return i, nil
		} else if i, err = parseFlagValue(reflect.Float64, v); err == nil {
			return i, nil
		} else if i, err = parseFlagValue(reflect.Bool, v); err == nil {
			return i, nil
		} else {
			return v, nil
		}
	default:
		return nil, fmt.Errorf("unsupported type %s, only integer, float, boolean and string is allowed", kind)
	}
}

func findField(t reflect.Type, v reflect.Value, name string) (rfield reflect.Value, filedName string, err error) {
	numField := t.NumField()
	mention, found := false, false
	for i := range numField {
		field := t.Field(i)
		tag := field.Tag.Get("flag")
		if !found && strings.HasPrefix(tag, name) {
			icomma := strings.IndexByte(tag, ',')
			if (icomma != -1 && tag[:icomma] != name) || (icomma == -1 && tag != name) {
				continue
			}
			rfield = v.Field(i)
			filedName = field.Name
			if mention {
				break
			}
			found = true
		} else if !mention && field.Tag.Get("mention") == name {
			if field.Type.Kind() != reflect.Bool {
				err = fmt.Errorf("mention field %s must have boolean type", field.Name)
				break
			}
			v.Field(i).SetBool(true)
			if found {
				break
			}
			mention = true
		}
	}
	return
}

func setDefaultFieldValue(t reflect.Type, v reflect.Value) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("flag")
		icomma := strings.IndexByte(tag, ',')
		if icomma > 0 {
			if defaultVal, err := parseFlagValue(field.Type.Kind(), tag[icomma+1:]); err != nil {
				return fmt.Errorf("invalid default value %s : %w", tag[icomma+1:], err)
			} else {
				v.Field(i).Set(reflect.ValueOf(defaultVal))
			}
		}
	}
	return nil
}
