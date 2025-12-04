package tests

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/cozees/cook/pkg/cook/parser"
	"github.com/cozees/cook/pkg/runtime/args"
	"github.com/cozees/cook/pkg/runtime/function"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// short term use as test prefix to avoid using regular expression when want to run uncompress binary test
// UB - stand for Uncompress Binary
// CB - stand for Compressed Binary

const (
	cookRawExec     = "cookraw"
	cookCompressExe = "cook"
)

var once sync.Once
var cdir, _ = os.Getwd()

var pathSeparator = fmt.Sprintf("%c", os.PathSeparator)
var pathListSeparator = fmt.Sprintf("%c", os.PathListSeparator)

func executableName(s string) string {
	if runtime.GOOS == "windows" {
		s += ".exe"
	}
	return s
}

func TestMain(m *testing.M) {
	if cdir == "" {
		fmt.Fprintln(os.Stderr, "cannot get current working directory.")
		os.Exit(1)
	}
	if err := buildNative(); err != nil {
		cleanup()
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	cpath := os.Getenv("PATH")
	if strings.HasSuffix(cpath, pathListSeparator) {
		cpath = cpath[:len(cpath)-1]
	}
	os.Setenv("PATH", fmt.Sprintf("%s%c%s", cpath, os.PathListSeparator, cdir))
	// run test, compress test will wait for the lock to be release after compress finish
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func cleanup() {
	os.Remove(executableName(cookRawExec))
	os.Remove(executableName(cookCompressExe))
	os.Remove(sreplaceFile1)
	os.Remove(sreplaceFile2)
	cleanPopulatedFDSample()
}

func buildNative() error {
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", executableName(cookRawExec), "../cmd/main.go", "../cmd/help.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ensureCompressBinary() {
	// run external compress on goroutine and let normal test run right away.
	// any error occurred then the build be can cell
	wait := &sync.WaitGroup{}
	wait.Go(func() {
		defer wait.Done()
		once.Do(func() {
			cmd := exec.Command("upx", "--brute", "-o", executableName(cookCompressExe), executableName(cookRawExec))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				cleanup()
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
		})
	})
	wait.Wait()
}

func TestSyntax(t *testing.T) {
	opts, err := args.ParseMainArgument([]string{})
	require.NoError(t, err)
	p := parser.NewParser()
	_, err = p.Parse(opts.Cookfile)
	require.NoError(t, err)
}

type testInput struct {
	args   []string
	output string
	file   string
}

var testVariableCases = []*testInput{
	{ // case 1
		args:   []string{"--INPUT", "text", "testVar"},
		output: "initialized\ntext Text text false false texttrue text Text text false false text Text text false false texttrue 12 text\nfinalized\n",
	},
	{ // case 2
		file:   "result",
		args:   []string{"--COND:b", "true", "--INPUT", "any", "--DATA", "mydata", "--OUTPUT", "result", "testIfElse"},
		output: "mydata",
	},
	{ // case 3
		args:   []string{"--COND:b", "true", "--INPUT", "", "--DATA", "zero-size-input", "testIfElse"},
		output: "initialized\nzero-size-inputfinalized\n",
	},
	{ // case 4
		args:   []string{"--COND:b", "false", "--INPUT", "123", "--DATA", "small-input", "testIfElse"},
		output: "initialized\nsmall-inputfinalized\n",
	},
	{ // case 5
		file:   "result",
		args:   []string{"--COND:b", "false", "--INPUT", "1xu2423", "--DATA", "small-input", "--OUTPUT", "result", "testIfElse"},
		output: "small-input",
	},
	{ // case 6
		args:   []string{"--CASE:i", "1", "--INPUT:i", "1001", "testFor"},
		output: "initialized\n1 1001\nfinalized\n",
	},
	{ // case 7
		args:   []string{"--CASE:i", "1", "--INPUT:i", "101", "testFor"},
		output: "initialized\n1 1101\nfinalized\n",
	},
	{ // case 8
		args:   []string{"--CASE:i", "2", "--LIST1:i", "2", "--LIST1:f", "3.2", "--LIST1:i", "11", "--LIST2:i", "2", "--LIST2:f", "3.2", "testFor"},
		output: "initialized\n8780\nfinalized\n",
	},
	{ // case 9
		args:   []string{"--CASE:i", "2", "--LIST1:i", "213", "--LIST1:i", "1", "--LIST1:i", "31", "--LIST2:i", "12", "--LIST2", "3.2", "testFor"},
		output: "initialized\n2059\nfinalized\n",
	},
	{ // case 10
		args:   []string{"--CASE:i", "3", "--LIST:i", "87", "--LIST:i", "21", "testFor"},
		output: "initialized\n0 87\n1 21\nfinalized\n",
	},
	{ // case 11
		// map is random order in go and since we directly run our statement base on Go map it is also
		// mean that order for in is also random so only given 1 item so no chance of randome issue
		args:   []string{"--CASE:i", "4", "--MAP:i:s", "34:abc", "testFor"},
		output: "initialized\n34 abc\nfinalized\n",
	},
}

func TestUBVariable(t *testing.T) {
	testWithCase(t, "TestVariable", cookRawExec, "uncompress binary", testVariableCases, nil)
}

func TestCBVariable(t *testing.T) {
	ensureCompressBinary()
	testWithCase(t, "TestVariable", cookCompressExe, "compressed binary", testVariableCases, nil)
}

func getTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		for k, vs := range r.Header {
			if strings.HasPrefix(k, "X-") || (r.Body != nil && k == "Content-Type") {
				for _, v := range vs {
					rw.Header().Add("R-"+k, v)
				}
			}
		}
		rw.Header().Set("R-Method", r.Method)
		switch r.Method {
		case http.MethodHead, http.MethodTrace:
			// do nothig
		case http.MethodPut:
			// don't reflect body but add it to header assuming test out have little body data
			defer r.Body.Close()
			b, err := io.ReadAll(r.Body)
			if err != nil {
				panic(err)
			}
			rw.Header().Set("R-Body", string(b))
		case http.MethodGet, http.MethodOptions:
			rw.Write([]byte("Body: TEXT"))
		default:
			// reflect body back
			if r.Body != nil && r.Body != http.NoBody {
				defer r.Body.Close()
				if _, err := io.Copy(rw, r.Body); err != nil {
					panic(err)
				}
			}
		}
	}))
}

var dataFile = "sample-http-result"
var dataSample = `{"sample": 123}`
var testHttpCases = []*testInput{
	{
		args:   []string{"@get"},
		output: "Body: TEXT\n",
	},
	{
		args:   []string{"@fetch"},
		output: "Body: TEXT\n",
	},
	{
		args:   []string{"@get", "-h", "X-Sample:123", "--header", "X-Sample:abc", "-h", "X-Version:1.23.3"},
		output: "Body: TEXT\n",
	},
	{
		args:   []string{"@head"},
		output: "\n",
	},
	{
		args:   []string{"@options"},
		output: "Body: TEXT\n",
	},
	{
		args:   []string{"@post", "-d", "X-Version:1.23.3"},
		output: "X-Version:1.23.3\n",
	},
	{
		args:   []string{"@post", "-f", dataFile},
		output: dataSample + "\n",
	},
	{
		args:   []string{"@patch", "-f", dataFile},
		output: dataSample + "\n",
	},
	{
		args:   []string{"@put", "-f", dataFile},
		output: "\n",
	},
	{
		args:   []string{"@delete", "-f", dataFile},
		output: dataSample + "\n",
	},
}

func addUrl(server *httptest.Server) func([]string) []string {
	return func(s []string) (r []string) {
		r = make([]string, len(s))
		copy(r, s)
		return append(r, server.URL)
	}
}

func TestUBHttp(t *testing.T) {
	require.NoError(t, os.WriteFile(dataFile, []byte(dataSample), 0700))
	defer os.Remove(dataFile)
	testWithCase(t, "TestHttp", cookRawExec, "uncompress binary", testHttpCases, addUrl(getTestServer()))
}

func TestCBHttp(t *testing.T) {
	ensureCompressBinary()
	require.NoError(t, os.WriteFile(dataFile, []byte(dataSample), 0700))
	defer os.Remove(dataFile)
	testWithCase(t, "TestHttp", cookCompressExe, "compressed binary", testHttpCases, addUrl(getTestServer()))
}

var testStringCases = []*testInput{
	{ // case 1
		args:   []string{"@spad", "-l", "5", "-r", "2", "--by", "??", "aa"},
		output: "??????????aa????\n",
	},
	{ // case 2
		args:   []string{"@spad", "-l", "5", "--by", "?", "aa"},
		output: "?????aa\n",
	},
	{ // case 3
		args:   []string{"@spad", "-l", "5", "-r", "2", "-m", "14", "--by", "??", "aa"},
		output: "??????????aa??\n",
	},
	{ // case 4
		args:   []string{"@spad", "-r", "8", "-m", "10", "--by", "?", "aa"},
		output: "aa????????\n",
	},
	{ // case 5
		args:   []string{"@spad", "-r", "8", "-m", "9", "--by", "?", "aa"},
		output: "aa???????\n",
	},
	{ // case 6
		args:   []string{"@spad", "-r", "4", "-m", "9", "--by", "??", "aa"},
		output: "aa???????\n",
	},
	{ // case 7
		args:   []string{"@ssplit", "--ws", "-l", "--rc", "1:0", "abc 123\n822 974"},
		output: "822\n",
	},
	{ // case 8
		args:   []string{"@ssplit", "--ws", "-l", "--rc", "-1:0", "abc 123\n822 974"},
		output: "822\n",
	},
	{ // case 9
		args:   []string{"@ssplit", "--ws", "-l", "--rc", "1", "abc 123\n822 974"},
		output: "[822, 974]\n",
	},
	{ // case 10
		args:   []string{"@ssplit", "--ws", "-l", "--rc", ":1", "abc 123\n822 974"},
		output: "[123, 974]\n",
	},
	{ // case 11
		args:   []string{"@ssplit", "--ws", "-l", "abc 123\n822 974"},
		output: "[[abc, 123], [822, 974]]\n",
	},
	{ // case 12
		args:   []string{"@ssplit", "--by", " ", "-l", "abc 123\n822 974"},
		output: "[[abc, 123], [822, 974]]\n",
	},
	{ // case 13
		args:   []string{"@ssplit", "--by", " ", "-l", "--rc", "-1", "abc 123\n822 974"},
		output: "[822, 974]\n",
	},
	{ // case 14
		args:   []string{"@ssplit", "--by", " ", "-l", "--rc", ":-1", "abc 123\n822 974"},
		output: "[123, 974]\n",
	},
	{ // case 15
		args:   []string{"@sreplace", "ax", "xa", "Lorem Ipsum is simply dummy text of the printing and typesetting industry"},
		output: "Lorem Ipsum is simply dummy text of the printing and typesetting industry\n",
	},
	{ // case 16
		args:   []string{"@sreplace", "ex", "xe", "Lorem Ipsum is simply dummy text of the printing and typesetting industry"},
		output: "Lorem Ipsum is simply dummy txet of the printing and typesetting industry\n",
	},
	{ // case 17
		args:   []string{"@sreplace", "-x", "ex?", "xe", "Lorem Ipsum is simply dummy text of the printing and typesetting industry"},
		output: "Lorxem Ipsum is simply dummy txet of thxe printing and typxesxetting industry\n",
	},
	{ // case 18
		args:   []string{"@sreplace", "-x", "-l", "1,2", "ex?", "xe", "Lorem Ipsum\n is simply dummy text\n of the printing and\n typesetting industry"},
		output: "Lorem Ipsum\n is simply dummy txet\n of thxe printing and\n typesetting industry\n",
	},
	{ // case 19
		args:   []string{"@sreplace", "-l", "1,2", "ex?", "xe", "@" + sreplaceFile1},
		output: sreplaceOriginal,
		file:   sreplaceFile1,
	},
	{ // case 20
		args:   []string{"@sreplace", "-x", "-l", "1,2", "ex?", "xe", "@" + sreplaceFile2, "@sreplace.modified"},
		output: sreplaceX,
		file:   "sreplace.modified",
	},
}

var sreplaceFile1 = "sreplace.original1"
var sreplaceFile2 = "sreplace.original2"
var sreplaceOriginal = `Lorem Ipsum is simply dummy text of the printing and typesetting industry.
Lorem Ipsum has been the industry's standard dummy text ever since the 1500s, when an unknown printer
took a galley of type and scrambled it to make a type specimen book. It has survived not only five
centuries, but also the leap into electronic typesetting, remaining essentially unchanged.
It was popularised in the 1960s with the release of Letraset sheets containing Lorem Ipsum passages,
and more recently with desktop publishing software like Aldus PageMaker including versions of Lorem Ipsum.`

// ex? expression
var sreplaceX = `Lorem Ipsum is simply dummy text of the printing and typesetting industry.
Lorxem Ipsum has bxexen thxe industry's standard dummy txet xevxer sincxe thxe 1500s, whxen an unknown printxer
took a gallxey of typxe and scramblxed it to makxe a typxe spxecimxen book. It has survivxed not only fivxe
centuries, but also the leap into electronic typesetting, remaining essentially unchanged.
It was popularised in the 1960s with the release of Letraset sheets containing Lorem Ipsum passages,
and more recently with desktop publishing software like Aldus PageMaker including versions of Lorem Ipsum.`

func TestUBString(t *testing.T) {
	require.NoError(t, os.WriteFile(sreplaceFile1, []byte(sreplaceOriginal), 0700))
	require.NoError(t, os.WriteFile(sreplaceFile2, []byte(sreplaceOriginal), 0700))
	defer os.Remove(sreplaceFile2)
	testWithCase(t, "TestStrings", cookRawExec, "uncompress binary", testStringCases, nil)
}

func TestCBString(t *testing.T) {
	ensureCompressBinary()
	require.NoError(t, os.WriteFile(sreplaceFile1, []byte(sreplaceOriginal), 0700))
	require.NoError(t, os.WriteFile(sreplaceFile2, []byte(sreplaceOriginal), 0700))
	defer os.Remove(sreplaceFile2)
	testWithCase(t, "TestStrings", cookCompressExe, "compressed binary", testStringCases, nil)
}

var allGoFile, _ = filepath.Glob(osPathTransform("./*.go"))

func osPathTransform(s string) string {
	if s[0] == '/' && runtime.GOOS == "windows" {
		return strings.ReplaceAll(s, "/", pathSeparator)
	}
	return strings.ReplaceAll(s, "/", pathSeparator)
}

func aboslutePath(s string) string {
	as, err := filepath.Abs(s)
	if err != nil {
		panic(err)
	}
	return as
}

var testPathCases = []*testInput{
	{ // case 1
		args:   []string{"@pabs", osPathTransform("test/abc/text.txt")},
		output: osPathTransform(fmt.Sprintf("%s/test/abc/text.txt\n", cdir)),
	},
	{ // case 2
		args:   []string{"@pabs", osPathTransform("/usr/abc/text.txt")},
		output: aboslutePath("/usr/abc/text.txt") + "\n",
	},
	{ // case 3
		args:   []string{"@pbase", osPathTransform("test/aa/bb")},
		output: "bb\n",
	},
	{ // case 4
		args:   []string{"@pbase", osPathTransform("test/aa/bb.txt")},
		output: "bb.txt\n",
	},
	{ // case 5
		args:   []string{"@pext", osPathTransform("test/aa/bb")},
		output: "\n",
	},
	{ // case 6
		args:   []string{"@pext", osPathTransform("test/aa/bb.txt")},
		output: ".txt\n",
	},
	{ // case 7
		args:   []string{"@pdir", osPathTransform("test/aa/bb.txt")},
		output: osPathTransform("test/aa\n"),
	},
	{ // case 8
		args:   []string{"@pclean", osPathTransform("abc/two/../test/./aa/bb.txt")},
		output: osPathTransform("abc/test/aa/bb.txt\n"),
	},
	{ // case 9
		args:   []string{"@psplit", osPathTransform("abc/two/../test/./aa/bb.txt")},
		output: "[abc, two, .., test, ., aa, bb.txt]\n",
	},
	{ // case 10
		args:   []string{"@prel", osPathTransform("/test/abc/test"), osPathTransform("/test/abc/two/../test/./aa/bb.txt")},
		output: osPathTransform("aa/bb.txt\n"),
	},
	{ // case 11
		args:   []string{"@pglob", osPathTransform("./*.go")},
		output: fmt.Sprintf("[%s]\n", strings.Join(allGoFile, ", ")),
	},
}

func TestUBPath(t *testing.T) {
	testWithCase(t, "TestPath", cookRawExec, "uncompress binary", testPathCases, nil)
}

func TestCBPath(t *testing.T) {
	ensureCompressBinary()
	testWithCase(t, "TestPath", cookCompressExe, "compressed binary", testPathCases, nil)
}

func testWithCase(t *testing.T, testName, xname, exeType string, cases []*testInput, fn func([]string) []string) {
	for i, tc := range cases {
		t.Logf("%s %s case #%d", testName, exeType, i+1)
		args := tc.args
		if fn != nil {
			args = fn(tc.args)
		}
		errBuf := bytes.NewBufferString("")
		cmd := exec.Command(executableName(xname), args...)
		cmd.Stderr = errBuf
		buf, err := cmd.Output()
		if err != nil || errBuf.Len() > 0 {
			fmt.Println(errBuf.String())
			t.Logf("ARGS: %s", strings.Join(tc.args, " "))
		}
		require.NoError(t, err)
		if tc.file != "" {
			buf, err = os.ReadFile(tc.file)
			os.Remove(tc.file)
			require.NoError(t, err)
		}
		assert.Equal(t, tc.output, string(buf))
	}
}

func TestUBFileDir(t *testing.T) {
	testFileDir(t, cookRawExec, "uncompress binary")
}

func TestCBFileDir(t *testing.T) {
	ensureCompressBinary()
	testFileDir(t, cookCompressExe, "compressed binary")
}

var pdir1Root = "s1"
var pdir3Root = "s2"
var pdir1 = filepath.Join(pdir1Root, "s2", "s3")
var pdir2 = filepath.Join(pdir1Root, "a", "b")
var pdir3 = filepath.Join(pdir3Root, "c")
var fileSet1 = []string{
	filepath.Join(pdir1Root, "s2", "f1.txt"),
	filepath.Join(pdir1Root, "s2", "f2.txt"),
	filepath.Join(pdir1Root, "s2", "f3.txt"),
	filepath.Join(pdir1Root, "s2", "f4.txt"),
}
var fileSet2 = []string{
	filepath.Join(pdir3Root, "c", "f1.txt"),
	filepath.Join(pdir3Root, "c", "f2.txt"),
}
var fileContents = []string{
	`Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor
incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud
exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.`,
	`Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium doloremque
laudantium, totam rem aperiam, eaque ipsa quae ab illo inventore veritatis et quasi
architecto beatae vitae dicta sunt explicabo.`,
	`At vero eos et accusamus et iusto odio dignissimos ducimus qui blanditiis praesentium
voluptatum deleniti atque corrupti quos dolores et quas molestias excepturi sint
occaecati cupiditate non provident`,
}

func populateFDSample(t *testing.T) {
	require.NoError(t, os.MkdirAll(pdir1, 0700))
	require.NoError(t, os.MkdirAll(pdir2, 0700))
	for i, content := range fileContents {
		require.NoError(t, os.WriteFile(fileSet1[i], []byte(content), 0700))
		if runtime.GOOS == "windows" {
			function.Chmod(fileSet1[i], "0700")
		}
	}
}

func cleanPopulatedFDSample() {
	os.RemoveAll(pdir1Root)
	os.RemoveAll(pdir3Root)
}

func ensurePermission(f string, m os.FileMode) error {
	if mode, err := function.GetFDModePerm(f); err != nil {
		return err
	} else if mode != m {
		return fmt.Errorf("file/directory %s expected %s but got %s", f, m, mode)
	}
	return nil
}

func testFileDir(t *testing.T, xname, exeType string) {
	run := func(isErr bool, args ...string) {
		cmd := exec.Command(executableName(xname), args...)
		buf := bytes.NewBufferString("")
		cmd.Stderr = buf
		cmd.Stdout = buf
		err := cmd.Run()
		if isErr {
			require.Error(t, err)
		} else if !assert.NoError(t, err) {
			fmt.Fprintln(os.Stderr, buf.String())
			t.FailNow()
		}
	}
	// test mkdir & rmdir
	t.Logf("TestFileDir %s mkdir & rmdir", exeType)
	dir1 := "p1"
	dir2 := filepath.Join("p1", "p2", "p3")
	run(false, "@mkdir", dir1)
	assert.DirExists(t, dir1)
	run(true, "@mkdir", dir2)
	assert.NoDirExists(t, dir2)
	run(false, "@mkdir", "-p", dir2)
	assert.DirExists(t, dir2)
	run(true, "@rmdir", dir1)
	assert.DirExists(t, dir2)
	run(false, "@rmdir", dir2)
	assert.NoDirExists(t, dir2)
	run(false, "@rmdir", "-p", dir1)
	assert.NoDirExists(t, dir1)
	// test check dir
	t.Logf("TestFileDir %s working or chdir", exeType)
	assert.NotEmpty(t, cdir)
	run(false, "@workin", os.TempDir())
	run(false, "@chdir", cdir)
	run(true, "@chdir", "not_exist/dir")
	// test rm
	t.Logf("TestFileDir %s rm", exeType)
	populateFDSample(t)
	run(true, "@rm", pdir1)
	assert.DirExists(t, pdir1)
	run(false, "@rm", fileSet1[0], fileSet1[2])
	assert.NoFileExists(t, fileSet1[0])
	assert.NoFileExists(t, fileSet1[2])
	run(false, "@rm", "-r", pdir1)
	assert.NoDirExists(t, pdir1)
	cleanPopulatedFDSample()
	// test mv
	t.Logf("TestFileDir %s mv or move", exeType)
	populateFDSample(t)
	run(false, "@mv", fileSet1[0], fileSet1[3])
	assert.NoFileExists(t, fileSet1[0])
	assert.FileExists(t, fileSet1[3])
	s4 := filepath.Join(pdir1Root, "s2", "s4")
	run(false, "@move", pdir1, s4)
	assert.NoDirExists(t, pdir1)
	assert.DirExists(t, s4)
	run(true, "@mv", fileSet1[3], fileSet2[0])
	require.NoError(t, os.MkdirAll(pdir3, 0700))
	run(false, "@mv", fileSet1[3], fileSet2[0])
	assert.FileExists(t, fileSet2[0])
	run(false, "@mv", filepath.Join(pdir1Root, "s2"), pdir3)
	assert.DirExists(t, filepath.Join(pdir3, "s2"))
	cleanPopulatedFDSample()
	// test cp & copy
	t.Logf("TestFileDir %s mv or move", exeType)
	populateFDSample(t)
	require.NoError(t, os.MkdirAll(pdir3, 0700))
	run(false, "@cp", fileSet1[0], fileSet2[0])
	assert.FileExists(t, fileSet1[0])
	assert.FileExists(t, fileSet2[0])
	cpDir := filepath.Join(pdir3, "s2")
	run(true, "@copy", filepath.Join(pdir1Root, "s2"), pdir3)
	assert.NoDirExists(t, cpDir)
	run(false, "@copy", "-r", filepath.Join(pdir1Root, "s2"), pdir3)
	assert.DirExists(t, cpDir)
	cleanPopulatedFDSample()
	// test chown
	populateFDSample(t)
	t.Logf("TestFileDir %s change owner", exeType)
	iuser, err := user.Current()
	require.NoError(t, err)
	igroup, err := user.LookupGroupId(iuser.Gid)
	require.NoError(t, err)
	run(true, "@chown", "-n", "abc:xyz", fileSet1[0])
	run(true, "@chown", "I0e0_abc:I0e0_xyz", fileSet1[0])
	run(true, "@chown", "I0e0_abc:I0e0_xyz", fileSet2[0])
	run(true, "@chown", fmt.Sprintf("%s:%s", iuser.Username, igroup.Name), fileSet2[0])
	run(false, "@chown", fmt.Sprintf("%s:%s", iuser.Username, igroup.Name), fileSet1[0])
	run(false, "@chown", fmt.Sprintf("%s:%s", iuser.Username, igroup.Name), pdir1)
	run(false, "@chown", "-r", fmt.Sprintf("%s:%s", iuser.Username, igroup.Name), pdir1)
	run(false, "@chown", "-r", "-n", fmt.Sprintf("%s:%s", iuser.Uid, igroup.Gid), pdir1)
	// test chmod
	t.Logf("TestFileDir %s change mode", exeType)
	require.NoError(t, ensurePermission(fileSet1[0], 0700))
	run(true, "@chmod", "0700", fileSet2[0])
	run(true, "@chmod", "0xz0", fileSet1[0])
	require.NoError(t, ensurePermission(fileSet1[0], 0700))
	run(false, "@chmod", "0755", fileSet1[0])
	require.NoError(t, ensurePermission(fileSet1[0], 0755))
	run(false, "@chmod", "-r", "0755", pdir1)
	require.NoError(t, filepath.Walk(pdir1, func(path string, info fs.FileInfo, err error) error {
		return ensurePermission(path, 0755)
	}))
	run(false, "@chmod", "-r", "u-w,go-x", pdir1)
	require.NoError(t, filepath.Walk(pdir1, func(path string, info fs.FileInfo, err error) error {
		return ensurePermission(path, 0544)
	}))
	cleanPopulatedFDSample()
}
