package args

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type OptionsTest struct {
	Flaga      string         `flag:"flaga"`
	IsMentionA bool           `mention:"flaga"`
	Flagb      bool           `flag:"flagb"`
	Flagc      int64          `flag:"flagc"`
	Flagd      float64        `flag:"flagd"`
	Flage      []int64        `flag:"flage"`
	Flagf      []any          `flag:"flagf"`
	Flagg      map[string]any `flag:"flagg"`
	Header     http.Header    `flag:"header"`
	Mslice     map[any][]any  `flag:"mslice"`
	Args       []any
}

const dummyDescription = `Lorem Ipsum is simply dummy text of the printing and typesetting industry.
Lorem Ipsum has been the industry's standard dummy text ever since the 1500s,
when an unknown printer took a galley of type and scrambled it to make a type specimen book.`

var testFlags = &Flags{
	Flags: []*Flag{
		{Short: "a", Long: "flaga"},
		{Short: "b", Long: "flagb"},
		{Short: "c", Long: "flagc"},
		{Short: "d", Long: "flagd"},
		{Short: "e", Long: "flage"},
		{Short: "f", Long: "flagf"},
		{Short: "g", Long: "flagg"},
		{Short: "h", Long: "header"},
		{Short: "m", Long: "mslice"},
	},
	Result: reflect.TypeOf((*OptionsTest)(nil)).Elem(),
}

type argsCase struct {
	input   []string
	opts    any
	failure bool
	err     error
}

var testCases = []*argsCase{
	{
		input: []string{"-b", "-a", "text"},
		opts: &OptionsTest{
			Flagb:      true,
			Flaga:      "text",
			IsMentionA: true,
		},
	},
	{
		input: []string{"-b", "-a", ""},
		opts: &OptionsTest{
			Flagb:      true,
			Flaga:      "",
			IsMentionA: true,
		},
	},
	{
		input: []string{"--flagb", "-a", "text", "non-flag-or-options"},
		opts: &OptionsTest{
			Flagb:      true,
			Flaga:      "text",
			IsMentionA: true,
			Args:       []any{"non-flag-or-options"},
		},
	},
	{
		input: []string{"--flaga", "discard text", "-a", "text", "non-flag-or-options"},
		opts: &OptionsTest{
			Flaga:      "text",
			IsMentionA: true,
			Args:       []any{"non-flag-or-options"},
		},
	},
	{
		input: []string{"-c", "873", "-a", "text", "non-flag-or-options"},
		opts: &OptionsTest{
			Flaga:      "text",
			IsMentionA: true,
			Flagc:      int64(873),
			Args:       []any{"non-flag-or-options"},
		},
	},
	{
		input: []string{"-c", "12", "--flagc", "873", "-a", "text of text", "non-flag-or-options"},
		opts: &OptionsTest{
			Flaga:      "text of text",
			IsMentionA: true,
			Flagc:      int64(873),
			Args:       []any{"non-flag-or-options"},
		},
	},
	{
		input: []string{"-d", "1.2", "non1", "non2", "--flage", "22", "-e", "42", "non3"},
		opts: &OptionsTest{
			Flagd: 1.2,
			Flage: []int64{22, 42},
			Args:  []any{"non1", "non2", "non3"},
		},
	},
	{
		input: []string{"--flagf", "99", "-f", "2.2", "-f", "text", "non3"},
		opts: &OptionsTest{
			Flagf: []any{int64(99), 2.2, "text"},
			Args:  []any{"non3"},
		},
	},
	{
		input: []string{"--flagg", "99:99", "-g", "2.2:abc", "-g", "text:2.3", "non3"},
		opts: &OptionsTest{
			Flagg: map[string]any{"99": int64(99), "2.2": "abc", "text": 2.3},
			Args:  []any{"non3"},
		},
	},
	{
		input: []string{"-h", "123:abc", "-h", "123:3.42", "--header", "abc:99312"},
		opts: &OptionsTest{
			Header: map[string][]string{
				"123": {"abc", "3.42"},
				"abc": {"99312"},
			},
		},
	},
	{
		input: []string{"--header", "abc:99312", "-m", "abc:123", "-m", "123:false", "-m", "123:1.23", "-m", "true:xyz"},
		opts: &OptionsTest{
			Header: map[string][]string{
				"abc": {"99312"},
			},
			Mslice: map[any][]any{
				"abc":      {int64(123)},
				int64(123): {false, 1.23},
				true:       {"xyz"},
			},
		},
	},
}

func TestFlag(t *testing.T) {
	for i, tc := range testCases {
		t.Logf("TestFlag case #%d", i+1)
		opts, err := testFlags.Parse(tc.input)
		require.NoError(t, err)
		assert.Equal(t, tc.opts, opts)
	}
}

type testFnFlag struct {
	input []*FunctionArg
	opts  any
}

var testFnCases = []*testFnFlag{
	{
		input: []*FunctionArg{
			{Val: "-b", Kind: reflect.String},
			{Val: "-a", Kind: reflect.String},
			{Val: "text", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Flagb:      true,
			Flaga:      "text",
			IsMentionA: true,
		},
	},
	{
		input: []*FunctionArg{
			{Val: "-b", Kind: reflect.String},
			{Val: "-a", Kind: reflect.String},
			{Val: "", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Flagb:      true,
			Flaga:      "",
			IsMentionA: true,
		},
	},
	{
		input: []*FunctionArg{
			{Val: "--flagb", Kind: reflect.String},
			{Val: "-a", Kind: reflect.String},
			{Val: "text", Kind: reflect.String},
			{Val: "non-flag-or-options", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Flagb:      true,
			Flaga:      "text",
			IsMentionA: true,
			Args:       []any{"non-flag-or-options"},
		},
	},
	{
		input: []*FunctionArg{
			{Val: "--flaga", Kind: reflect.String},
			{Val: "discard text", Kind: reflect.String},
			{Val: "-a", Kind: reflect.String},
			{Val: "text", Kind: reflect.String},
			{Val: "non-flag-or-options", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Flaga:      "text",
			IsMentionA: true,
			Args:       []any{"non-flag-or-options"},
		},
	},
	{
		input: []*FunctionArg{
			{Val: "-c", Kind: reflect.String},
			{Val: int64(873), Kind: reflect.Int64},
			{Val: "-a", Kind: reflect.String},
			{Val: "text", Kind: reflect.String},
			{Val: "non-flag-or-options", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Flaga:      "text",
			IsMentionA: true,
			Flagc:      int64(873),
			Args:       []any{"non-flag-or-options"},
		},
	},
	{
		input: []*FunctionArg{
			{Val: "-c", Kind: reflect.String},
			{Val: int64(12), Kind: reflect.Int64},
			{Val: "--flagc", Kind: reflect.String},
			{Val: int64(873), Kind: reflect.Int64},
			{Val: "-a", Kind: reflect.String},
			{Val: "text of text", Kind: reflect.String},
			{Val: "non-flag-or-options", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Flaga:      "text of text",
			IsMentionA: true,
			Flagc:      int64(873),
			Args:       []any{"non-flag-or-options"},
		},
	},
	{
		input: []*FunctionArg{
			{Val: "-d", Kind: reflect.String},
			{Val: 1.2, Kind: reflect.Float64},
			{Val: "non1", Kind: reflect.String},
			{Val: "non2", Kind: reflect.String},
			{Val: "--flage", Kind: reflect.String},
			{Val: int64(22), Kind: reflect.Int64},
			{Val: "-e", Kind: reflect.String},
			{Val: int64(42), Kind: reflect.Int64},
			{Val: "non3", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Flagd: 1.2,
			Flage: []int64{22, 42},
			Args:  []any{"non1", "non2", "non3"},
		},
	},
	{
		input: []*FunctionArg{
			{Val: "--flagf", Kind: reflect.String},
			{Val: int64(99), Kind: reflect.Int64},
			{Val: "-f", Kind: reflect.String},
			{Val: 2.2, Kind: reflect.Float64},
			{Val: "-f", Kind: reflect.String},
			{Val: "text", Kind: reflect.String},
			{Val: "non3", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Flagf: []any{int64(99), 2.2, "text"},
			Args:  []any{"non3"},
		},
	},
	{
		input: []*FunctionArg{
			{Val: "--flagg", Kind: reflect.String},
			{Val: map[string]int64{"99": int64(99)}, Kind: reflect.Map},
			{Val: "-g", Kind: reflect.String},
			{Val: map[string]string{"2.2": "abc"}, Kind: reflect.Map},
			{Val: "-g", Kind: reflect.String},
			{Val: "text:2.3", Kind: reflect.String},
			{Val: "non3", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Flagg: map[string]any{"99": int64(99), "2.2": "abc", "text": 2.3},
			Args:  []any{"non3"},
		},
	},
	{
		input: []*FunctionArg{
			{Val: "-h", Kind: reflect.String},
			{Val: "123:abc", Kind: reflect.String},
			{Val: "-h", Kind: reflect.String},
			{Val: "123:3.42", Kind: reflect.String},
			{Val: "--header", Kind: reflect.String},
			{Val: "abc:99312", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Header: map[string][]string{
				"123": {"abc", "3.42"},
				"abc": {"99312"},
			},
		},
	},
	{
		input: []*FunctionArg{
			{Val: "--header", Kind: reflect.String},
			{Val: "abc:99312", Kind: reflect.String},
			{Val: "-m", Kind: reflect.String},
			{Val: "abc:123", Kind: reflect.String},
			{Val: "-m", Kind: reflect.String},
			{Val: "123:false", Kind: reflect.String},
			{Val: "-m", Kind: reflect.String},
			{Val: "123:1.23", Kind: reflect.String},
			{Val: "-m", Kind: reflect.String},
			{Val: "true:xyz", Kind: reflect.String},
		},
		opts: &OptionsTest{
			Header: map[string][]string{
				"abc": {"99312"},
			},
			Mslice: map[any][]any{
				"abc":      {int64(123)},
				int64(123): {false, 1.23},
				true:       {"xyz"},
			},
		},
	},
	{
		input: []*FunctionArg{
			{Val: "--header", Kind: reflect.String},
			{Val: "abc:99312", Kind: reflect.String},
			{Val: "-m", Kind: reflect.String},
			{
				Val: map[any][]any{
					"xyz": {int64(852), 1.6, "ioy"},
					1.45:  {"hol", int64(93), true},
				},
				Kind: reflect.Map,
			},
		},
		opts: &OptionsTest{
			Header: map[string][]string{
				"abc": {"99312"},
			},
			Mslice: map[any][]any{
				"xyz": {int64(852), 1.6, "ioy"},
				1.45:  {"hol", int64(93), true},
			},
		},
	},
}

func TestFuncFlagParsing(t *testing.T) {
	for i, tc := range testFnCases {
		t.Logf("TestFnParsing case #%d", i+1)
		opts, err := testFlags.ParseFunctionArgs(tc.input)
		require.NoError(t, err)
		assert.Equal(t, tc.opts, opts)
	}
}

var testMainFlagCases = []*argsCase{
	{
		input: []string{"--name", "nyu", "target"},
		opts: &MainOptions{
			Cookfile: defaultCookfile,
			Args: map[string]any{
				"name": "nyu",
			},
			Targets: []string{"target"},
		},
	},
	{
		input: []string{"--age:i", "32", "--age:i", "34", "--height:f", "1.57"},
		opts: &MainOptions{
			Cookfile: defaultCookfile,
			Args: map[string]any{
				"age":    []any{int64(32), int64(34)},
				"height": 1.57,
			},
		},
	},
	{
		input: []string{"--age:s", "32", "--age:i", "34", "--info:a:i", "3.21:33", "--info:s:i", "8.23:33", "--info:s:a", "bb:32.1"},
		opts: &MainOptions{
			Cookfile: defaultCookfile,
			Args: map[string]any{
				"age": []any{"32", int64(34)},
				"info": map[any]any{
					3.21:   int64(33),
					"8.23": int64(33),
					"bb":   32.1,
				},
			},
		},
	},
	{
		input: []string{"sample1", "sample2", "-c", "Cooksample", "--name=test", "--info:i=123", "--lerp:a", "3.21"},
		opts: &MainOptions{
			Cookfile: "Cooksample",
			Args: map[string]any{
				"name": "test",
				"info": int64(123),
				"lerp": 3.21,
			},
			Targets: []string{"sample1", "sample2"},
		},
	},
	// test error
	{
		input:   []string{"--dict:a", "22", "--dict:i:s", "11:aa"},
		failure: true,
	},
	{
		input:   []string{"--dict:a:a", "22"},
		failure: true,
	},
	{
		input:   []string{"--val:i", "22", "9038"},
		failure: true,
	},
	{
		input:   []string{"--dict:o", "22"},
		failure: true,
		err:     ErrAllowFormat,
	},
	{
		input:   []string{"--dict:", "22"},
		failure: true,
		err:     ErrFlagSyntax,
	},
}

func TestMainFlag(t *testing.T) {
	for i, tc := range testMainFlagCases {
		t.Logf("TestMainFlag case #%d", i+1)
		opts, err := ParseMainArgument(tc.input)
		if tc.failure {
			if tc.err != nil {
				assert.ErrorIs(t, tc.err, err)
			} else {
				assert.Error(t, err)
			}
		} else {
			require.NoError(t, err)
			assert.Equal(t, tc.opts, opts)
		}
	}
}
