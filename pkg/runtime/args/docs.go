package args

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"

	"golang.org/x/term"
)

type stage uint8

const (
	name stage = iota
	usage
	description
	flag
	example
)

var stageName = [...]string{
	name:        "Name",
	usage:       "Usage",
	description: "Description",
	flag:        "Flag",
	example:     "Example",
}

func (s stage) String() string { return stageName[s] }

type FlagWriter func(maxFlagSize int, short, long, defaultVal, description string)

type Builder interface {
	io.Reader
	Name(n, shortDesc string, aliases ...string)
	Usage(s string)
	Description(s string)
	Flag(flag []*Flag, t reflect.Type)
	FlagVisitor(fn func(fw FlagWriter))
	Example(s, topAnchor string)
	String() string
}

func NewMarkdownBuilder() Builder { return &mdb{buf: bytes.NewBufferString("")} }

type mdb struct {
	buf   *bytes.Buffer
	stage stage
}

func (b *mdb) ensureStage(s stage) {
	if s != b.stage {
		panic(fmt.Sprintf("required providing %s before %s", b.stage, s))
	}
}

func (b *mdb) Read(p []byte) (n int, err error) { return b.buf.Read(p) }

func (b *mdb) Name(n string, shortDesc string, aliases ...string) {
	b.ensureStage(name)
	b.buf.WriteString("## @")
	b.buf.WriteString(n)
	for _, alias := range aliases {
		b.buf.WriteString(", @")
		b.buf.WriteString(alias)
	}
	b.buf.WriteByte('\n')
	b.stage++
}

func (b *mdb) Usage(s string) {
	b.ensureStage(usage)
	b.buf.WriteString("\nUsage:\n")
	b.buf.WriteString("```cook\n")
	b.buf.WriteString(s)
	b.buf.WriteString("\n```\n")
	b.stage++
}

func (b *mdb) Description(s string) {
	b.ensureStage(description)
	b.buf.WriteByte('\n')
	b.buf.WriteString(whitespace.Replace(strings.TrimSpace(s)))
	b.buf.WriteByte('\n')
	b.stage++
}

func (b *mdb) Flag(flags []*Flag, t reflect.Type) {
	b.FlagVisitor(func(fw FlagWriter) {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		for _, fl := range flags {
			def := ""
		fieldLookup:
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				if dflag, ok := field.Tag.Lookup("flag"); ok && dflag == fl.Long {
					def, ok = field.Tag.Lookup("default")
					if kind := field.Type.Kind(); kind == reflect.Bool {
						// boolean default value is always false
						def = "false"
						break fieldLookup
					} else if !ok {
						switch kind {
						case reflect.Int64:
							def = "0"
						case reflect.Float64:
							def = "0.0"
						case reflect.String:
							def = `""`
						case reflect.Slice, reflect.Map:
							def = "nil"
						}
					}
					break
				}
			}
			fw(0, fl.Short, fl.Long, def, fl.Description)
		}
	})
}

func (b *mdb) FlagVisitor(fn func(fw FlagWriter)) {
	b.ensureStage(flag)
	b.buf.WriteString("\n| Options/Flag | Default | Description |\n")
	b.buf.WriteString("| --- | --- | --- |\n")
	fn(b.visitFlags)
	b.stage++
}

func (b *mdb) visitFlags(maxFlagSize int, short, long, defaultVal, description string) {
	b.buf.WriteString("| ")
	if short != "" {
		b.buf.WriteByte('-')
		b.buf.WriteString(short)
		b.buf.WriteString(", ")
	}
	b.buf.WriteString("--")
	b.buf.WriteString(long)
	// write default, require type from field defined in struct.
	// a boolean does not required extra value
	b.buf.WriteString(" | ")
	b.buf.WriteString(defaultVal)
	b.buf.WriteString(" | ")
	// description last
	b.buf.WriteString(whitespace.Replace(strings.TrimSpace(description)))
	b.buf.WriteString(" |\n")
}

func (b *mdb) Example(s, topAnchor string) {
	b.ensureStage(example)
	b.buf.WriteString("\nExample:\n")
	b.buf.WriteString("\n```cook\n")
	b.buf.WriteString(s)
	b.buf.WriteString("\n```\n")
	if topAnchor != "" {
		fmt.Fprintf(b.buf, "[back top](#%s)\n", topAnchor)
	}
	b.stage++
}

func (b *mdb) String() string { return b.buf.String() }

func NewConsoleBuilder() Builder {
	w, _, err := term.GetSize(0)
	if err != nil {
		w = 80
	}
	return &console{buf: bytes.NewBufferString(""), width: w}
}

type console struct {
	buf   *bytes.Buffer
	stage stage
	width int
}

func (b *console) ensureStage(s stage) {
	if s != b.stage {
		panic(fmt.Sprintf("required providing %s before %s", b.stage, s))
	}
}

func (b *console) addIndent(i int) { b.buf.WriteString(strings.Repeat(" ", i*4)) }

func (b *console) Read(p []byte) (n int, err error) { return b.buf.Read(p) }

func (b *console) Name(n, shortDesc string, aliases ...string) {
	b.ensureStage(name)
	b.buf.WriteString("NAME\n")
	b.addIndent(1)
	b.buf.WriteString(n)
	for _, alias := range aliases {
		b.buf.WriteString(", ")
		b.buf.WriteString(alias)
	}
	b.buf.WriteString(" -- ")
	b.buf.WriteString(shortDesc)
	b.buf.WriteString("\n\n")
	b.stage++
}

func (b *console) Usage(s string) {
	b.ensureStage(usage)
	b.buf.WriteString("USAGE\n")
	b.addIndent(1)
	b.buf.WriteString(wrapTextByLine(4, s))
	b.buf.WriteString("\n\n")
	b.stage++
}

func (b *console) Description(s string) {
	b.ensureStage(description)
	b.buf.WriteString("DESCRIPTION\n")
	b.buf.WriteString(wrapTextWith(4, 0, b.width, s))
	b.buf.WriteByte('\n')
	b.stage++
}

func (b *console) Flag(flags []*Flag, t reflect.Type) {
	mw := maxWidthFlag(flags)
	b.FlagVisitor(func(fw FlagWriter) {
		for _, fl := range flags {
			fw(mw, fl.Short, fl.Long, "", fl.Description)
		}
	})
}

func (b *console) FlagVisitor(fn func(fw FlagWriter)) {
	b.ensureStage(flag)
	b.buf.WriteString("\nAvailable options or flag:\n\n")
	fn(b.visitFlags)
	b.stage++
}

func (b *console) visitFlags(maxFlagSize int, short, long, defaultVal, description string) {
	b.addIndent(1)
	w := 0
	if short != "" {
		b.buf.WriteByte('-')
		b.buf.WriteString(short)
		b.buf.WriteString(", ")
		w += 4
	}
	b.buf.WriteString("--")
	b.buf.WriteString(long)
	w += 2 + len(long)
	if maxFlagSize > w {
		b.buf.WriteString(strings.Repeat(" ", maxFlagSize-w))
	}
	// description last
	b.buf.WriteString(wrapTextWith(6, 4+maxFlagSize, b.width, description))
	b.buf.WriteString("\n\n")
}

func (b *console) Example(s, _ string) {
	b.ensureStage(example)
	b.buf.WriteString("EXAMPLE\n")
	b.addIndent(1)
	b.buf.WriteString(wrapTextByLine(4, s))
	b.buf.WriteByte('\n')
	b.buf.WriteByte('\n')
	b.stage++
}

func (b *console) String() string { return b.buf.String() }

func maxWidthFlag(flags []*Flag) int {
	w, cur := 0, 0
	for _, flag := range flags {
		cur = 0
		if flag.Short != "" {
			// "-e, " 4 chars
			cur += 4
		}
		// "--long"
		cur += 2 + len(flag.Long)
		if w < cur {
			w = cur
		}
	}
	return w
}

var whitespace = strings.NewReplacer("\n", " ", "\t", " ")

func wrapTextByLine(space int, txt string) string {
	buf := strings.Builder{}
	indent := strings.Repeat(" ", space)
	count, ln := 0, 0
	for _, c := range txt {
		switch c {
		case ' ', '\t':
			if count == 0 {
				buf.WriteByte(' ')
			}
			count++
		case '\n':
			buf.WriteRune(c)
			if ln == 0 {
				buf.WriteString(indent)
			}
			ln++
			count = 1
		default:
			count, ln = 0, 0
			buf.WriteRune(c)
		}
	}
	return buf.String()
}

func wrapTextWith(initSpace, space, w int, txt string) string {
	buf := strings.Builder{}
	start, next, seg := 0, w, ""
	buf.WriteString(strings.Repeat(" ", initSpace))
	space += initSpace
	w -= space
	writer := func(s string) {
		count := 0
		for _, c := range s {
			switch c {
			case ' ', '\t', '\n':
				if count == 0 {
					buf.WriteRune(c)
				}
				count++
			default:
				count = 0
				buf.WriteRune(c)
			}
		}
	}
	if w > len(txt) {
		w = len(txt)
	}
	for {
		if start != 0 {
			buf.WriteString(strings.Repeat(" ", space))
		}
		i := strings.LastIndexAny(txt[:start+w], " \t\n")
		if i == -1 {
			next--
			seg = whitespace.Replace(strings.TrimSpace(txt[start:next]))
			start, next = next, next+w
		} else {
			seg = whitespace.Replace(strings.TrimSpace(txt[start:i]))
			start, next = i, i+w
		}
		writer(seg)
		buf.WriteByte('\n')
		if next > len(txt) {
			buf.WriteString(strings.Repeat(" ", space))
			writer(whitespace.Replace(strings.TrimSpace((txt[start:]))))
			break
		}
	}
	return buf.String()
}
