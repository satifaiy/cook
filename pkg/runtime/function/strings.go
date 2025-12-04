package function

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cozees/cook/pkg/runtime/args"
	"github.com/cozees/cook/pkg/runtime/parser"
)

func AllStringFlags() []*args.Flags {
	return []*args.Flags{sreplaceFlags, ssplitFlags, spadFlags}
}

type sSplitOption struct {
	WS   bool   `flag:"ws"`
	Line bool   `flag:"line"`
	By   string `flag:"by"`
	Regx string `flag:"regx"`
	RC   string `flag:"rc"`
	Args []string
}

func (so *sSplitOption) rowColumn() (row, column int, err error) {
	row, column = -2, -2
	if so.RC == "" {
		return
	}
	index, i := strings.IndexByte(so.RC, ':'), int64(-1)
	if index == 0 {
		if i, err = strconv.ParseInt(so.RC[1:], 10, 32); err != nil {
			return
		}
		column = int(i)
	} else if index > 0 {
		if i, err = strconv.ParseInt(so.RC[:index], 10, 32); err != nil {
			return
		}
		row = int(i)
		if i, err = strconv.ParseInt(so.RC[index+1:], 10, 32); err != nil {
			return
		}
		column = int(i)
	} else if i, err = strconv.ParseInt(so.RC, 10, 32); err == nil {
		row = int(i)
	}
	return
}

const (
	wsDesc = `Tell @ssplit to split the string by any whitespace character.
			  If flag --line is given then @ssplit will split each line into row result in table instead of array.`
	lineDesc = `Tell @ssplit to split the string by line into string array or table depend on flag --ws.`
	byDesc   = `Tell @ssplit to split the string into array or table by given string. If flag --by is space it is
				similar to flag --ws except that it ignore other whitespace character such newline or tab. If flag
				--ws and --by is given at the same time then @ssplit will ignore flag --by.`
	regxDesc = `Tell @ssplit to split the string into array using the given regular expression. Split with Regular
	 			Expression does not support split by line flag thus it only output array of string. If flag --regx
				is given then other flag will be ignored.`
	rcDesc = `Tell @ssplit to return a single string at given row and column instead of array or table. The flag --rc
			  use conjunction with other flag, for example, if a row value is given then it's also required flag --line
			  to be given as well otherwise @ssplit will return an error instead.`
	ssplitDesc = `Split a string into array or table depend on the given flag. The split function required input to be
				  a regular string or a unicode string, a redirect syntax to split a non-text file will result with
				  unknown behavior.`
)

var ssplitFlags = &args.Flags{
	Flags: []*args.Flag{
		{Long: "ws", Description: wsDesc},
		{Short: "l", Long: "line", Description: lineDesc},
		{Long: "by", Description: byDesc},
		{Long: "regx", Description: regxDesc},
		{Long: "rc", Description: rcDesc},
	},
	Result:      reflect.TypeOf((*sSplitOption)(nil)).Elem(),
	FuncName:    "ssplit",
	ShortDesc:   "split string into column and row",
	Usage:       "@ssplit [-l] [--ws] [--by value] [--regx expression] [--rc row:column] STRING",
	Example:     "// result in table or array 2 dimension [[a,b,c],[d,e,f]]\n@ssplit --ws -l \"a b c\nd e f\"",
	Description: ssplitDesc,
}

type sReplaceOption struct {
	Regx bool   `flag:"regx"` // true if search is regular expression
	Line string `flag:"line"` // a line to replace, format single line numer 1 or multiple line with comma 1,1,11
	Args []any
}

const (
	replaceRegxDesc = `Tell @sreplace that the first argument is a regular expression rather than a normal string.
	                   Also note that when first argument is a regular expression then second argument can also use
					   regular expression variable (${number}) as the replacement as well.`
	replaceLineDesc = `Tell @sreplace to only replace the old string with a new string a the given line. Multiple can
					   be given with comma (,) separation. If the given line is greater than the line count in the given
					   string then nothing is being replace and no new file is being created.`
	sreplaceDesc = `Replace a string of the first given argument in a file or a given string with
					a new string given by the second arguments. If the given old string is an empty
					string then the new string will be place after each unicode character in the given
					string. If the fourth argument is given it must begin with an @ character to indicate
					that the replacement should written to that file instead regardless if the third
					argument is a string or a file which also begin with an @. Note: when replace the string
					by regular expression, the function @sreplace replace each string by line instead of a while
					file.`
)

var sreplaceFlags = &args.Flags{
	Flags: []*args.Flag{
		{Short: "x", Long: "regx", Description: replaceRegxDesc},
		{Short: "l", Long: "line"},
	},
	Result:    reflect.TypeOf((*sReplaceOption)(nil)).Elem(),
	FuncName:  "sreplace",
	ShortDesc: "replace string in a string or a file",
	Usage:     `@sreplace [-x] [--line value,...] {regular|string} {replacement} STRING [@OUT]`,
	Example: `@sreplace -l 1 sample elpmas "sample text"
			  @sreplace -x -l 1,2,3 "(\d+)x" "0x${1}" @file
			  @sreplace -x "(\d+)x" "0x${1}" @file @out`,
	Description: sreplaceDesc,
}

const bufferSize = 2048

func replaceString(src, search, replace string) (pending, result string) {
	n := strings.Count(src, search)
	if n == 0 {
		if len(src) < len(search) {
			return src, ""
		} else {
			l := len(src) - len(search)
			return src[l:], src[:l]
		}
	}
	var b strings.Builder
	acc := len(src)
	b.Grow(acc + n*(len(replace)-len(search)))
	start := 0
	for i := range n {
		j := start
		if len(search) == 0 {
			if i > 0 {
				_, wid := utf8.DecodeRuneInString(src[start:])
				j += wid
			}
		} else {
			j += strings.Index(src[start:], search)
		}
		b.WriteString(src[start:j])
		b.WriteString(replace)
		start = j + len(search)
	}
	minial := acc - len(search)
	if start < minial {
		b.WriteString(src[start:minial])
		start = minial
	}
	return src[start:], b.String()
}

type sPadOptions struct {
	Left  int64  `flag:"left"`
	Right int64  `flag:"right"`
	By    string `flag:"by"`
	Max   int64  `flag:"max"`
	Args  []any
}

const (
	spadLeftDesc = `number of string to be pads left of a string. It is number of time a string given with flag
	                --by to be repeated and concatenate to left.`
	spadRightDesc = `number of string to be pads right of a string. It is number of time a string given with flag
					 --by to be repeated and concatenate to right.`
	spadMaxDesc = `A total maximum number of character allowed. This number of unicode character is compare with the padding result.`
	spadByDesc  = `The string which use for padding, if it is empty or not given then the original argument is return instead.`
	spadDesc    = `Pads the given string with another string given by "--by" flag until the resulting string is satisfied the
				   given number to left and the right or it reach the maximum length. If number of total character exceeded the maximum
				   given by --max flag then the result will be truncated.`
)

var spadFlags = &args.Flags{
	Flags: []*args.Flag{
		{Short: "l", Long: "left", Description: spadLeftDesc},
		{Short: "r", Long: "right", Description: spadRightDesc},
		{Short: "m", Long: "max", Description: spadMaxDesc},
		{Long: "by", Description: spadByDesc},
	},
	Result:      reflect.TypeOf((*sPadOptions)(nil)).Elem(),
	FuncName:    "spad",
	ShortDesc:   "Pads a string by the another string",
	Usage:       "@spad [--left value] [--right value] [--max value] [--by value] STRING",
	Example:     "@spad -l 5 -r 5 -m 12 --by 0 ii",
	Description: spadDesc,
}

func pad(left, right, max int, by, arg string) string {
	if max > 0 && len(arg) >= max {
		return arg
	}
	// reduce left and right paddingt o fit max length
	cut := 0
	if max > 0 {
		a := false
		cut = left*len(by) + right*len(by) + len(arg) - max
		for cut >= len(by) {
			if a && left > 0 {
				left--
			} else if right > 0 {
				right--
			}
			a = !a
			cut = left*len(by) + right*len(by) + len(arg) - max
		}
	}
	if left > 0 {
		arg = strings.Repeat(by, left) + arg
		if cut > 0 {
			arg = arg[cut:]
			cut = 0
		}
	}
	if right > 0 {
		arg += strings.Repeat(by, right)
		if cut > 0 {
			arg = arg[:len(arg)-cut]
		}
	}
	return arg
}

func init() {
	registerFunction(NewBaseFunction(spadFlags, func(f Function, i any) (any, error) {
		opts := i.(*sPadOptions)
		switch si := len(opts.Args); si {
		case 0:
			return nil, fmt.Errorf("spad required at least once argument")
		case 1:
			if opts.Left == 0 && opts.Right == 0 {
				return opts.Args[0], nil
			}
			s, err := toString(opts.Args[0])
			if err != nil {
				return nil, err
			}
			return pad(int(opts.Left), int(opts.Right), int(opts.Max), opts.By, s), nil
		default:
			if opts.Left == 0 && opts.Right == 0 {
				return opts.Args, nil
			}
			result := make([]string, si)
			for i, arg := range opts.Args {
				s, err := toString(arg)
				if err != nil {
					return nil, err
				}
				result[i] = pad(int(opts.Left), int(opts.Right), int(opts.Max), opts.By, s)
			}
			return result, nil
		}
	}))

	registerFunction(NewBaseFunction(ssplitFlags, func(f Function, i any) (any, error) {
		opts := i.(*sSplitOption)
		// validate the argument
		if len(opts.Args) > 1 || len(opts.Args) == 0 {
			return nil, fmt.Errorf("split required a single ascii or unicode utf-8 argument")
		} else if opts.Line && opts.By != "" && strings.ContainsAny(opts.By, "\n\r") {
			return nil, fmt.Errorf("flag \"-by\" must not contain any newline character if flag \"--line\" is given")
		}
		// regular expression is produce array of string only
		sarg := opts.Args[0]
		if opts.Regx != "" {
			reg, err := regexp.Compile(opts.Regx)
			if err != nil {
				return nil, fmt.Errorf("invalid regular expression %s: %w", opts.Regx, err)
			}
			return reg.Split(sarg, -1), nil
		} else if opts.By != "" || opts.WS || opts.Line {
			row, col, err := opts.rowColumn()
			if err != nil {
				return nil, err
			} else if !opts.Line && (row != -2 || col != -2) {
				return nil, fmt.Errorf("flag \"--line\" is required when specified flag \"--rc\"")
			}
			s := parser.NewSimpleScanner(false)
			s.Init([]byte(sarg))
			lastNLPos, lastLine := strings.LastIndexByte(sarg, '\n'), false
			offs, cur, cr, cc := 0, 0, 0, 0
			var result, array []any
			var seg string
			isCell := row != -2 && col != -2
			var asg func(nl, last bool, s string) bool
			if !isCell {
				result = make([]any, 0, len(sarg))
				array = make([]any, 0, 1)
				asg = func(nl, last bool, s string) bool {
					if row != -2 || col != -2 {
						if row == cr || (last && row == -1) || col == cc || (nl && col == -1) {
							result = append(result, s)
							return row == cr && nl
						}
					} else if opts.Line {
						array = append(array, s)
						if nl {
							if opts.WS || opts.By != "" {
								result = append(result, array)
							} else {
								result = append(result, array...)
							}
							array = make([]any, 0, 1)
						}
					} else {
						result = append(result, s)
					}
					return false
				}
			} else {
				asg = func(nl, last bool, _ string) bool {
					return (row == cr || (last && row == -1)) && (col == cc || (nl && col == -1))
				}
			}

			lno, bsno := 0, 0
		innerLoop:
			for ch, err := s.Next(); err == nil && ch != parser.RuneEOF; ch, err = s.Next() {
			revisit:
				cur = s.Offset()
				switch {
				case ch == '\r':
					if s.Peek() == '\n' {
						if _, err = s.Next(); err != nil {
							return nil, err
						}
					}
					fallthrough
				case ch == '\n':
					currentlyLastLine := lastLine
					lastLine = lastNLPos == cur
					if opts.Line {
						if lno == 0 {
							if seg = sarg[offs:cur]; asg(true, currentlyLastLine, seg) {
								goto conclude
							}
							cr++
							cc = 0
							bsno = 0
						}
						offs = s.NextOffset()
						lno++
						continue innerLoop
					}
					fallthrough
				case unicode.IsSpace(ch):
					if opts.WS {
						if bsno == 0 {
							if seg = sarg[offs:cur]; asg(false, lastLine, seg) {
								goto conclude
							}
							cc++
						}
						offs = s.NextOffset()
						bsno++
						break
					} else if strings.IndexRune(opts.By, ch) != 0 {
						break
					}
					fallthrough
				case strings.IndexRune(opts.By, ch) == 0:
					// only if it start with ch
					ni := utf8.RuneLen(ch)
					if ni == len(opts.By) {
						if bsno == 0 {
							if seg = sarg[offs:cur]; asg(false, lastLine, seg) {
								goto conclude
							}
							cc++
						}
						offs = s.NextOffset()
						bsno++
						continue innerLoop
					}
					for {
						if ch, err = s.Next(); err != nil {
							return nil, err
						} else if ni < len(opts.By) && strings.IndexRune(opts.By[ni:], ch) == 0 {
							ni += utf8.RuneLen(ch)
							if ni == len(opts.By) { // match delimiter "by"
								if bsno == 0 {
									if seg = sarg[offs:cur]; asg(false, lastLine, seg) {
										goto conclude
									}
									cc++
								}
								offs = s.NextOffset()
								bsno++
								continue innerLoop
							}
							continue
						}
						break
					}
					lno = 0
					bsno = 0
					if opts.Line && (ch == '\r' || ch == '\n') {
						goto revisit
					}
				default:
					lno = 0
					bsno = 0
				}
			}
			if offs < len(sarg) {
				// last segment follow by EOF and no newline.
				if seg = sarg[offs:]; asg(true, lastLine, seg) {
					goto conclude
				}
			}
			if len(array) > 0 {
				if opts.Line {
					result = append(result, array)
				} else {
					result = append(result, array...)
				}
			}
		conclude:
			if isCell {
				return seg, nil
			} else {
				return result, err
			}
		} else {
			return nil, fmt.Errorf("required at least one split flag available")
		}
	}))

	registerFunction(NewBaseFunction(sreplaceFlags, func(f Function, i any) (any, error) {
		opts := i.(*sReplaceOption)
		numArgs := len(opts.Args)
		if numArgs != 3 && numArgs != 4 {
			return nil, fmt.Errorf("required three or four argument")
		}
		// if search and replace value is the same then there is need to run replace function
		var ok bool
		inPlace := numArgs == 3
		sargs := make([]string, numArgs)
		for i := range numArgs {
			if sargs[i], ok = opts.Args[i].(string); !ok {
				return nil, fmt.Errorf("argument %v is not a string", opts.Args[i])
			}
		}
		// if replace old and new is the same then nothing need to repalce at all
		if sargs[0] == sargs[1] && !opts.Regx {
			if strings.HasPrefix(sargs[2], "@") {
				return nil, nil
			} else {
				return sargs[2], nil
			}
		}
		// if line available
		var lines []int
		if opts.Line != "" {
			if strings.IndexByte(opts.Line, ',') != -1 {
				for lstr := range strings.SplitSeq(opts.Line, ",") {
					l, err := strconv.ParseInt(lstr, 10, 32)
					if err != nil {
						return nil, fmt.Errorf("invalid line format %s from %s: %w", lstr, opts.Line, err)
					}
					lines = append(lines, int(l))
				}
				sort.Ints(lines)
			} else if l, err := strconv.ParseInt(opts.Line, 10, 32); err != nil {
				return nil, fmt.Errorf("invalid line format %s: %w", opts.Line, err)
			} else {
				lines = append(lines, int(l))
			}
		}
		// a file input
		if strings.HasPrefix(sargs[2], "@") {
			// result is alway written to a newfile
			var file = sargs[2][1:]
			var fileBC string
			if inPlace {
				fileBC = filepath.Join(os.TempDir(), ".cook-replace."+file)
				defer os.Remove(fileBC)
			} else {
				fileBC = sargs[3][1:]
			}
			// open file read & fileBC write
			f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0700)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			fstat, err := f.Stat()
			if err != nil {
				return nil, err
			}
			f2, err := os.OpenFile(fileBC, os.O_CREATE|os.O_RDWR, fstat.Mode())
			if err != nil {
				return nil, err
			}
			defer f2.Close()
			// this probably no perfect there is corner case there the unicode string was happen to split
			// at the end of bufferSize the regular expression or normal replace can be found
			byteCount, cce := int64(0), 0
			if opts.Regx {
				buf := bufio.NewReader(f)
				var reg *regexp.Regexp
				reg, err = regexp.Compile(sargs[0])
				if err != nil {
					return nil, err
				}
				line := 0
				for {
					src, errRead := buf.ReadString('\n')
					if src != "" {
						switch {
						case len(lines) > 0 && lines[0] == line:
							lines = lines[1:]
							fallthrough
						case lines == nil:
							if cce, err = f2.WriteString(reg.ReplaceAllString(src, sargs[1])); err != nil {
								return nil, err
							}
						default:
							if cce, err = f2.WriteString(src); err != nil {
								return nil, err
							}
						}
						line++
						byteCount += int64(cce)
					}
					if errRead == io.EOF {
						break
					}
					if errRead != nil {
						return nil, err
					}
				}
			} else {
				buf := [bufferSize]byte{}
				prev, result, n, line := "", "", 0, 0
				ireader := bufio.NewReader(f)
				var errRead error
				for {
					if len(lines) == 0 {
						n, errRead = ireader.Read(buf[:])
						if errRead != nil && errRead != io.EOF {
							return nil, err
						}
						if lines == nil {
							prev, result = replaceString(prev+string(buf[:n]), sargs[0], sargs[1])
						} else {
							result = string(buf[:n])
						}
						if cce, err = f2.WriteString(result); err != nil {
							return nil, err
						}
					} else {
						n = bufferSize
						lstr, errRead := ireader.ReadString('\n')
						if errRead != nil && errRead != io.EOF {
							return nil, err
						} else if err == io.EOF {
							n = 0
						}
						if lines[0] == line {
							if cce, err = f2.WriteString(strings.ReplaceAll(lstr, sargs[0], sargs[1])); err != nil {
								return nil, err
							}
							lines = lines[1:]
						} else if cce, err = f2.WriteString(lstr); err != nil {
							return nil, err
						}
						line++
					}
					byteCount += int64(cce)
					// n is less than bufferSize then there is no more data to read
					if errRead == io.EOF {
						if len(prev) > 0 {
							if cce, err = f2.WriteString(prev); err != nil {
								return nil, err
							}
							byteCount += int64(cce)
						}
						break
					}
				}
			}
			// copy content of new file to old if in place
			if inPlace {
				if err = f.Truncate(byteCount); err == nil {
					var exn int64
					if exn, err = f.Seek(0, 0); err == nil && exn == 0 {
						if exn, err = f2.Seek(0, 0); err == nil && exn == 0 {
							if exn, err = io.Copy(f, f2); exn == byteCount && err == nil {
								return nil, nil
							}
							err = fmt.Errorf("only %d out of %d byte(s) have been copied", exn, byteCount)
						}
					}
					if err == nil {
						err = fmt.Errorf("file seek offset expected 0 but get %d", exn)
					}
				}
			}
			return nil, err
		}
		if len(lines) > 0 {
			maxLine := strings.Count(sargs[2], "\n")
			// check if total line is less than minimum search&replace line thus there is nothing to replace
			if maxLine-1 < lines[0] {
				return sargs[2], nil
			}
			var reg *regexp.Regexp
			var err error
			if opts.Regx {
				reg, err = regexp.Compile(sargs[0])
				if err != nil {
					return nil, err
				}
			}
			offs, index := 0, 0
			buf := strings.Builder{}
			for i := range maxLine {
				// include newline too
				index = offs + strings.IndexByte(sargs[2][offs:], '\n') + 1
				if index == -1 {
					index = len(sargs[2])
				}
				if i == lines[0] {
					if opts.Regx {
						buf.WriteString(reg.ReplaceAllString(sargs[2][offs:index], sargs[1]))
					} else {
						buf.WriteString(strings.ReplaceAll(sargs[2][offs:index], sargs[0], sargs[1]))
					}
					lines = lines[1:]
				} else {
					buf.WriteString(sargs[2][offs:index])
				}
				offs = index
			}
			if offs < len(sargs[2]) {
				buf.WriteString(sargs[2][offs:])
			}
			return buf.String(), nil
		} else if opts.Regx {
			reg, err := regexp.Compile(sargs[0])
			if err != nil {
				return nil, err
			}
			return reg.ReplaceAllString(sargs[2], sargs[1]), nil
		} else {
			return strings.ReplaceAll(sargs[2], sargs[0], sargs[1]), nil
		}
	}))
}
