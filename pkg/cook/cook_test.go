package cook

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/cozees/cook/pkg/cook/ast"
	"github.com/cozees/cook/pkg/cook/parser"
	"github.com/cozees/cook/pkg/cook/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type integrateTestCase struct {
	name   string
	vname  string
	output string
}

var cases = []*integrateTestCase{
	{
		name:  "file1.txt",
		vname: "FILE1",
		output: `
0 2021 3.36.0
1 2020 3.32.0
2 2015 3.9.2
3 2012 3.1.2.1
data1 data2 data3
0 https://www.sqlite.org/2021/sqlite-amalgamation-3360000.zip
1 https://www.sqlite.org/2020/sqlite-amalgamation-3320000.zip
2 https://www.sqlite.org/2015/sqlite-amalgamation-3090200.zip
3 https://www.sqlite.org/2012/sqlite-amalgamation-3010201.zip
189 magic number
256 magic number
text 83 38
`,
	},
	{
		name:  "file2.txt",
		vname: "FILE2",
		output: `
123 abc
finalize executed
rmdir existed
moooh
cook_test.go
cook_test___.go not found
`,
	},
}

const nestedResult = "nasted-result.txt"

// 0: LIST, 1: MAP, 2: result
var nestLoopTestCase = [][]any{
	{
		[]any{int64(1), int64(2), int64(3)},
		[]any{int64(1), int64(2), 1.3},
	},
	{
		[]any{int64(11), int64(4), int64(9)},
		[]any{22.4, int64(2), 1.3},
	},
	{
		[]any{int64(11), int64(5), int64(12)},
		[]any{true, false, 1.3},
	},
	{
		[]any{int64(9), int64(15), int64(122)},
		[]any{1.3, int64(40), false},
	},
	{
		[]any{int64(5), int64(7), int64(32)},
		[]any{24.8, int64(4), int64(40)},
	},
}

func cleanup() {
	for _, tc := range cases {
		os.Remove(tc.name)
	}
	os.Remove(nestedResult)
}

func TestCookProgram(t *testing.T) {
	defer cleanup()
	p := parser.NewParser()
	cook, err := p.Parse("testdata/Cookfile")
	require.NoError(t, err)
	args := make(map[string]any)
	args["TEST_NEST_LOOP"] = false
	for _, tc := range cases {
		args[tc.vname] = tc.name
	}
	require.NoError(t, cook.Execute(args))
	for i, tc := range cases {
		t.Logf("TestCookProgram case #%d", i+1)
		bo, err := os.ReadFile(tc.name)
		assert.NoError(t, err)
		assert.Equal(t, tc.output, string(bo))
	}

	// test nested loop
	args["TEST_NEST_LOOP"] = true
	for i, tc := range nestLoopTestCase {
		t.Logf("TestCookProgram Nested Loop case #%d", i+1)
		args["LIST"] = tc[0]
		args["LISTA"] = tc[1]
		args["FILE1"] = nestedResult
		require.NoError(t, cook.ExecuteWithTarget(args, "sampleNestLoop"))
		bo, err := os.ReadFile(nestedResult)
		require.NoError(t, err)
		expectResult := resultNestLoop(tc[0].([]any), tc[1].([]any))
		require.Equal(t, expectResult, string(bo))
	}
}

func resultNestLoop(LIST, LISTA []any) string {
	a11 := 0
	b22 := 0
out1:
	for i := 1; i <= 200; i++ {
		if i < 100 {
			a11 += 1
		middle1:
			for iv, v := range LIST {
				if inv, ok := v.(int64); ok && inv > 10 {
					i += iv
				} else {
					a11 += 2
					for _, mv := range LISTA {
						if inmv, ok := mv.(int64); ok {
							if inmv > 30 {
								continue middle1
							}
							i += int(inmv)
							a11 += 4
						} else if fnmv, ok := mv.(float64); ok {
							if fnmv > 30 {
								continue middle1
							}
							i += int(fnmv)
							a11 += 4
						} else {
							continue out1
						}
					}
				}
			}
		} else {
			b22++
			i += 2
			if i%3 == 2 {
				a11++
				i++
			} else {
				a11 += 2
				i--
			}
		}
	}
	return fmt.Sprintf("%d %d", a11, b22)
}

type Source struct {
	src      string
	verifier func(t *testing.T, scope ast.Scope)
}

var stateCases = []*Source{
	{
		src: `
V = [[1,2],[3,4]]
for i, iv in V {
	iv += 'text'
}
all:
`,
		verifier: func(t *testing.T, scope ast.Scope) {
			v, vk, _ := scope.GetVariable("V")
			require.Equal(t, reflect.Slice, vk)
			assert.Equal(t, []any{
				[]any{int64(1), int64(2), "text"},
				[]any{int64(3), int64(4), "text"},
			}, v)
		},
	},
}

func TestExecuteState(t *testing.T) {
	for i, tc := range stateCases {
		t.Logf("TestExecuteState case #%d", i+1)
		p := parser.NewParser()
		c, err := p.ParseSrc(token.NewFile("sample", len(tc.src)), []byte(tc.src))
		require.NoError(t, err)
		require.NoError(t, c.Execute(nil))
		tc.verifier(t, c.Scope())
	}
}
