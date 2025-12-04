package function

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cozees/cook/pkg/runtime/args"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pathInOut struct {
	name   string
	args   []*args.FunctionArg
	output any
}

var pathTestCase []*pathInOut

func init() {
	var cdir, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	allGoFile, err := filepath.Glob("./*.go")
	if err != nil {
		panic(err)
	}
	root := "/"
	if runtime.GOOS == "windows" {
		root = "C:\\"
	}
	// We want to use filepath.Join unfortunately it call clean as well thus .. and . it being remove
	pathSample1 := strings.Join([]string{"abc", "two", "..", "test", ".", "aa", "bb.txt"}, fmt.Sprintf("%c", os.PathSeparator))
	pathTestCase = []*pathInOut{
		{ // case 1
			name:   "pabs",
			args:   convertToFunctionArgs([]string{filepath.Join("test", "abc", "text.txt")}),
			output: filepath.Join(cdir, "test", "abc", "text.txt"),
		},
		{ // case 2
			name:   "pabs",
			args:   convertToFunctionArgs([]string{root + filepath.Join("usr", "abc", "text.txt")}),
			output: root + filepath.Join("usr", "abc", "text.txt"),
		},
		{ // case 3
			name:   "pbase",
			args:   convertToFunctionArgs([]string{filepath.Join("test", "aa", "bb")}),
			output: "bb",
		},
		{ // case 4
			name:   "pbase",
			args:   convertToFunctionArgs([]string{filepath.Join("test", "aa", "bb.txt")}),
			output: "bb.txt",
		},
		{ // case 5
			name:   "pext",
			args:   convertToFunctionArgs([]string{filepath.Join("test", "aa", "bb")}),
			output: "",
		},
		{ // case 6
			name:   "pext",
			args:   convertToFunctionArgs([]string{filepath.Join("test", "aa", "bb.txt")}),
			output: ".txt",
		},
		{ // case 7
			name:   "pdir",
			args:   convertToFunctionArgs([]string{filepath.Join("test", "aa", "bb.txt")}),
			output: filepath.Join("test", "aa"),
		},
		{ // case 8
			name:   "pclean",
			args:   convertToFunctionArgs([]string{filepath.Join("abc", "two", "..", "test", ".", "aa", "bb.txt")}),
			output: filepath.Join("abc", "test", "aa", "bb.txt"),
		},
		{ // case 9
			name:   "psplit",
			args:   convertToFunctionArgs([]string{pathSample1}),
			output: []string{"abc", "two", "..", "test", ".", "aa", "bb.txt"},
		},
		{ // case 10
			name:   "prel",
			args:   convertToFunctionArgs([]string{root + filepath.Join("test", "abc", "test"), filepath.Join("abc", "two", "..", "test", ".", "aa", "bb.txt")}),
			output: nil,
		},
		{ // case 11
			name:   "prel",
			args:   convertToFunctionArgs([]string{root + filepath.Join("test", "abc", "test"), root + filepath.Join("test", "abc", "two", "..", "test", ".", "aa", "bb.txt")}),
			output: filepath.Join("aa", "bb.txt"),
		},
		{ // case 12
			name:   "pglob",
			args:   convertToFunctionArgs([]string{"./*.go"}),
			output: allGoFile,
		},
	}
}

func TestPathFunction(t *testing.T) {
	for i, tc := range pathTestCase {
		t.Logf("TestPath function %s case #%d", tc.name, i+1)
		fn := GetFunction(tc.name)
		result, err := fn.Apply(tc.args)
		if tc.output == nil {
			assert.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tc.output, result)
		}
	}
}
