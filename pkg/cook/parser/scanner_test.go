package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cozees/cook/pkg/cook/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scanOutput struct {
	offset int
	tok    token.Token
	lit    string
}

type testCase struct {
	src    string
	output []*scanOutput
}

var stringScanCase = []*testCase{
	{ // case 1
		src: "'sample test'",
		output: []*scanOutput{
			{tok: token.STRING, lit: "sample test"},
		},
	},
	{ // case 2
		src: "\"sample test + test\"",
		output: []*scanOutput{
			{tok: token.STRING, lit: "sample test + test"},
		},
	},
	{ // case 3
		src: "\"sample test $variable test\"",
		output: []*scanOutput{
			{tok: token.STRING_ITP, lit: "sample test "},
			{tok: token.VAR, lit: "$"},
			{tok: token.IDENT, lit: "variable"},
			{tok: token.STRING_ITP, lit: " test"},
		},
	},
	{ // case 4
		src: "\"sample * $test + test\"",
		output: []*scanOutput{
			{tok: token.STRING_ITP, lit: "sample * "},
			{tok: token.VAR, lit: "$"},
			{tok: token.IDENT, lit: "test"},
			{tok: token.STRING_ITP, lit: " + test"},
		},
	},
	{ // case 5
		src: "'sample * $test + test'",
		output: []*scanOutput{
			{tok: token.STRING_ITP, lit: "sample * "},
			{tok: token.VAR, lit: "$"},
			{tok: token.IDENT, lit: "test"},
			{tok: token.STRING_ITP, lit: " + test"},
		},
	},
	{ // case 6
		src: "`sample * $test + test`",
		output: []*scanOutput{
			{tok: token.STRING_ITP, lit: "sample * "},
			{tok: token.VAR, lit: "$"},
			{tok: token.IDENT, lit: "test"},
			{tok: token.STRING_ITP, lit: " + test"},
		},
	},
	{ // case 7
		src: "'sample * $test + test'\n\t\t'text second line'",
		output: []*scanOutput{
			{tok: token.STRING_ITP, lit: "sample * "},
			{tok: token.VAR, lit: "$"},
			{tok: token.IDENT, lit: "test"},
			{tok: token.STRING_ITP, lit: " + test"},
			{tok: token.LF, lit: "\n"},
			{tok: token.STRING, lit: "text second line"},
		},
	},
	{ // case 8
		src: "// single line comment",
		output: []*scanOutput{
			{tok: token.COMMENT, lit: "// single line comment"},
		},
	},
	{ // case 9
		src: "/* first line comment\nsecond line comment */",
		output: []*scanOutput{
			{tok: token.COMMENT, lit: "/* first line comment\nsecond line comment */"},
		},
	},
}

func TestStringScanner(t *testing.T) {
	var errs []error
	eh := func(p token.Position, msg string, args ...any) {
		errs = append(errs, fmt.Errorf(msg, args...))
	}
	for i, tc := range stringScanCase {
		t.Logf("TestStringScanner case #%d", i+1)
		s, err := NewScannerSrc(token.NewFile("sample", len(tc.src)), []byte(tc.src), eh)
		require.NoError(t, err)
		for o, out := range tc.output {
			t.Logf("TestStringScanner case #%d output #%d", i+1, o+1)
			_, tok, lit := s.Scan()
			assert.Equal(t, out.tok, tok)
			assert.Equal(t, out.lit, lit)
		}
	}
}

var sources = []*testCase{
	{
		src: "var = a",
		output: []*scanOutput{
			{tok: token.IDENT, lit: "var"},
			{tok: token.ASSIGN, lit: "="},
			{tok: token.IDENT, lit: "a"},
			{tok: token.LF, lit: "\n"},
		},
	},
	{
		src: "var = 12 * 12 + sample",
		output: []*scanOutput{
			{tok: token.IDENT, lit: "var"},
			{tok: token.ASSIGN, lit: "="},
			{tok: token.INTEGER, lit: "12"},
			{tok: token.MUL, lit: "*"},
			{tok: token.INTEGER, lit: "12"},
			{tok: token.ADD, lit: "+"},
			{tok: token.IDENT, lit: "sample"},
			{tok: token.LF, lit: "\n"},
		},
	},
	{
		src: "var = u12 * 12 + sample",
		output: []*scanOutput{
			{tok: token.IDENT, lit: "var"},
			{tok: token.ASSIGN, lit: "="},
			{tok: token.IDENT, lit: "u12"},
			{tok: token.MUL, lit: "*"},
			{tok: token.INTEGER, lit: "12"},
			{tok: token.ADD, lit: "+"},
			{tok: token.IDENT, lit: "sample"},
			{tok: token.LF, lit: "\n"},
		},
	},
	{
		src: "var = _uyo2 * 12 + 'sample 984 text multiple space' / 34",
		output: []*scanOutput{
			{tok: token.IDENT, lit: "var"},
			{tok: token.ASSIGN, lit: "="},
			{tok: token.IDENT, lit: "_uyo2"},
			{tok: token.MUL, lit: "*"},
			{tok: token.INTEGER, lit: "12"},
			{tok: token.ADD, lit: "+"},
			{tok: token.STRING, lit: "sample 984 text multiple space"},
			{tok: token.QUO, lit: "/"},
			{tok: token.INTEGER, lit: "34"},
			{tok: token.LF, lit: "\n"},
		},
	},
	{
		src: "target:", output: []*scanOutput{
			{tok: token.IDENT, lit: "target"},
			{tok: token.COLON, lit: ":"},
		},
	},
	{
		src: "\tfor i in 1..10 {", output: []*scanOutput{
			{tok: token.FOR, lit: "for"},
			{tok: token.IDENT, lit: "i"},
			{tok: token.IN, lit: "in"},
			{tok: token.INTEGER, lit: "1"},
			{tok: token.RANGE, lit: ".."},
			{tok: token.INTEGER, lit: "10"},
			{tok: token.LBRACE, lit: "{"},
		},
	},
	{
		src: "\tfor i in [1..10) {", output: []*scanOutput{
			{tok: token.FOR, lit: "for"},
			{tok: token.IDENT, lit: "i"},
			{tok: token.IN, lit: "in"},
			{tok: token.LBRACK, lit: "["},
			{tok: token.INTEGER, lit: "1"},
			{tok: token.RANGE, lit: ".."},
			{tok: token.INTEGER, lit: "10"},
			{tok: token.RPAREN, lit: ")"},
			{tok: token.LBRACE, lit: "{"},
		},
	},
	{
		src: "\tif a is integer | float {", output: []*scanOutput{
			{tok: token.IF, lit: "if"},
			{tok: token.IDENT, lit: "a"},
			{tok: token.IS, lit: "is"},
			{tok: token.TINTEGER, lit: "integer"},
			{tok: token.OR, lit: "|"},
			{tok: token.TFLOAT, lit: "float"},
			{tok: token.LBRACE, lit: "{"},
		},
	},
	{
		src: "\ta = [1,2,b,{1:2}]", output: []*scanOutput{
			{tok: token.IDENT, lit: "a"},
			{tok: token.ASSIGN, lit: "="},
			{tok: token.LBRACK, lit: "["},
			{tok: token.INTEGER, lit: "1"},
			{tok: token.COMMA, lit: ","},
			{tok: token.INTEGER, lit: "2"},
			{tok: token.COMMA, lit: ","},
			{tok: token.IDENT, lit: "b"},
			{tok: token.COMMA, lit: ","},
			{tok: token.LBRACE, lit: "{"},
			{tok: token.INTEGER, lit: "1"},
			{tok: token.COLON, lit: ":"},
			{tok: token.INTEGER, lit: "2"},
			{tok: token.RBRACE, lit: "}"},
			{tok: token.RBRACK, lit: "]"},
			{tok: token.LF, lit: "\n"},
		},
	},
	{
		src: "\tsample(a, b) {", output: []*scanOutput{
			{tok: token.IDENT, lit: "sample"},
			{tok: token.LPAREN, lit: "("},
			{tok: token.IDENT, lit: "a"},
			{tok: token.COMMA, lit: ","},
			{tok: token.IDENT, lit: "b"},
			{tok: token.RPAREN, lit: ")"},
			{tok: token.LBRACE, lit: "{"},
		},
	},
	{
		src: "\tdelete a{1..2}", output: []*scanOutput{
			{tok: token.DELETE, lit: "delete"},
			{tok: token.IDENT, lit: "a"},
			{tok: token.LBRACE, lit: "{"},
			{tok: token.INTEGER, lit: "1"},
			{tok: token.RANGE, lit: ".."},
			{tok: token.INTEGER, lit: "2"},
			{tok: token.RBRACE, lit: "}"},
			{tok: token.LF, lit: "\n"},
		},
	},
	{
		src: "\tA{0} += [1,2,3]", output: []*scanOutput{
			{tok: token.IDENT, lit: "A"},
			{tok: token.LBRACE, lit: "{"},
			{tok: token.INTEGER, lit: "0"},
			{tok: token.RBRACE, lit: "}"},
			{tok: token.ADD_ASSIGN, lit: "+="},
			{tok: token.LBRACK, lit: "["},
			{tok: token.INTEGER, lit: "1"},
			{tok: token.COMMA, lit: ","},
			{tok: token.INTEGER, lit: "2"},
			{tok: token.COMMA, lit: ","},
			{tok: token.INTEGER, lit: "3"},
			{tok: token.RBRACK, lit: "]"},
			{tok: token.LF, lit: "\n"},
		},
	},
	{
		src: "\tif a < b { }", output: []*scanOutput{
			{tok: token.IF, lit: "if"},
			{tok: token.IDENT, lit: "a"},
			{tok: token.LSS, lit: "<"},
			{tok: token.IDENT, lit: "b"},
			{tok: token.LBRACE, lit: "{"},
			{tok: token.RBRACE, lit: "}"},
			{tok: token.LF, lit: "\n"},
		},
	},
	{
		src: "\t} else { }", output: []*scanOutput{
			{tok: token.RBRACE, lit: "}"},
			{tok: token.ELSE, lit: "else"},
			{tok: token.LBRACE, lit: "{"},
			{tok: token.RBRACE, lit: "}"},
			{tok: token.LF, lit: "\n"},
		},
	},
	{
		src: "\t(a, b) => a * b", output: []*scanOutput{
			{tok: token.LPAREN, lit: "("},
			{tok: token.IDENT, lit: "a"},
			{tok: token.COMMA, lit: ","},
			{tok: token.IDENT, lit: "b"},
			{tok: token.RPAREN, lit: ")"},
			{tok: token.LAMBDA, lit: "=>"},
			{tok: token.IDENT, lit: "a"},
			{tok: token.MUL, lit: "*"},
			{tok: token.IDENT, lit: "b"},
			{tok: token.LF, lit: "\n"},
		},
	},
}

var source string
var output []*scanOutput

func init() {
	builder := &strings.Builder{}
	offset := 0
	for _, tc := range sources {
		builder.WriteString(tc.src)
		builder.WriteByte('\n')
		offs := 0
		for _, out := range tc.output {
			if out.tok == token.LF {
				out.offset = builder.Len() - 1
			} else {
				ind := offs + strings.Index(tc.src[offs:], out.lit)
				out.offset = offset + ind
				offs = ind
			}
		}
		offset = builder.Len()
		output = append(output, tc.output...)
	}
	source = builder.String()
}

func TestGeneralScanner(t *testing.T) {
	var errs []error
	eh := func(p token.Position, msg string, args ...any) {
		errs = append(errs, fmt.Errorf(msg, args...))
	}
	s, err := NewScannerSrc(token.NewFile("sample", len(source)), []byte(source), eh)
	require.NoError(t, err)
	i := 0
	for offset, tok, lit := s.Scan(); tok != token.EOF; offset, tok, lit = s.Scan() {
		t.Logf("TestGeneralScanner token #%d", i+1)
		assert.Equal(t, output[i].tok, tok)
		assert.Equal(t, output[i].lit, lit)
		assert.Equal(t, output[i].offset, offset)
		i++
	}
}
