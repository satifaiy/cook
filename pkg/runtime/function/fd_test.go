package function

import (
	"fmt"
	"os"
	osu "os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cozees/cook/pkg/runtime/args"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fmpCase struct {
	origin os.FileMode
	input  string
	ouput  string
	goout  string // os.FileMode.String(), it's produce different unix pattern when user and group execute id set
}

var fmpTestCase = []*fmpCase{
	{origin: 0, input: "+x", ouput: "---x--x--x"},                          // origin: ----------
	{origin: 0, input: "+rx", ouput: "-r-xr-xr-x"},                         // origin: ----------
	{origin: 0, input: "+rwx", ouput: "-rwxrwxrwx"},                        // origin: ----------
	{origin: 0500, input: "+rwx", ouput: "-rwxrwxrwx"},                     // origin: -r-x------
	{origin: 0500, input: "g=u", ouput: "-r-xr-x---"},                      // origin: -r-x------
	{origin: 0500, input: "g=uw", ouput: "-r-x-w----"},                     // origin: -r-x------
	{origin: 0500, input: "og=uw", ouput: "-r-x-w--w-"},                    // origin: -r-x------
	{origin: 0500, input: "og=u-x", ouput: "-r-xr--r--"},                   // origin: -r-x------
	{origin: 0777, input: "og=", ouput: "-rwx------"},                      // origin: -rwxrwxrwx
	{origin: 0700, input: "go+x", ouput: "-rwx--x--x"},                     // origin: -rwx------
	{origin: 0, input: "u=rwx", ouput: "-rwx------"},                       // origin: ----------
	{origin: 0, input: "u=rwx,go=rx", ouput: "-rwxr-xr-x"},                 // origin: ----------
	{origin: 0, input: "u=rwx,go=u-w", ouput: "-rwxr-xr-x"},                // origin: ----------
	{origin: 0, input: "u=rwx,go=u-x-w", ouput: "-rwxr--r--"},              // origin: ----------
	{origin: 0, input: "u=rwx,go=u-w-r", ouput: "-rwx--x--x"},              // origin: ----------
	{origin: 0510, input: "go=u-w-r", ouput: "-r-x--x--x"},                 // origin: -r-x--x---
	{origin: 0510, input: "go=u-w-r,u=rwx", ouput: "-rwx--x--x"},           // origin: -r-x--x---
	{origin: 0510, input: "u=rwx,go=u-w-r", ouput: "-rwx--x--x"},           // origin: -r-x--x---
	{origin: 0556, input: "=,+X", ouput: "---x--x--x"},                     // origin: -r-xr-xrw-
	{origin: 0666, input: "=,+X", ouput: "----------"},                     // origin: -rw-rw-rw-
	{origin: 0556, input: "a=,+X", ouput: "---x--x--x"},                    // origin: -r-xr-xrw-
	{origin: 0556, input: "ugo=,+X", ouput: "---x--x--x"},                  // origin: -r-xr-xrw-
	{origin: 0556, input: "+s", ouput: "-r-sr-srw-", goout: "ugr-xr-xrw-"}, // origin: -r-xr-xrw-
	{origin: 0666, input: "+s", ouput: "-rwSrwSrw-", goout: "ugrw-rw-rw-"}, // origin: -rw-rw-rw-
}

func TestFileModeParser(t *testing.T) {
	for i, tc := range fmpTestCase {
		t.Logf("Test case %d input %s", i+1, tc.input)
		m, err := fm.Parse(tc.origin, tc.input)
		assert.NoError(t, err)
		require.Equal(t, tc.ouput, UnixStringPermission(m, false))
		if tc.goout != "" {
			require.Equal(t, tc.goout, m.String())
		} else {
			require.Equal(t, tc.ouput, m.String())
		}
	}
}

func TestDir(t *testing.T) {
	path := "p1"
	pathRecur := filepath.Join("p1", "p2", "p3")
	// -p as recursive flag
	paths := []string{"-p", "p2", filepath.Join("d1", "d2", "d3"), filepath.Join("f1", "f3")}
	defer func() {
		// clean up
		os.RemoveAll(path)
		for _, p := range paths[1:] {
			os.RemoveAll(p)
		}
	}()
	expect := func(notExist bool, paths ...string) {
		for _, path := range paths {
			stat, err := os.Stat(path)
			if notExist {
				assert.ErrorIs(t, err, os.ErrNotExist)
				assert.Nil(t, stat)
			} else {
				assert.NoError(t, err)
				if assert.NotNil(t, stat) {
					assert.True(t, stat.IsDir())
				}
			}
		}
	}
	// get the function
	fmkdir := GetFunction("mkdir")
	frmdir := GetFunction("rmdir")
	assert.NotNil(t, fmkdir)
	// direct child directory
	expect(true, path)
	_, err := fmkdir.Apply(convertToFunctionArgs([]string{path}))
	assert.NoError(t, err)
	expect(false, path)
	// attempt to create directory on existing directory
	_, err = fmkdir.Apply(convertToFunctionArgs([]string{path}))
	assert.Error(t, err)
	// multi deep child directory required -p
	expect(true, pathRecur)
	_, err = fmkdir.Apply(convertToFunctionArgs([]string{pathRecur}))
	assert.Error(t, err)
	expect(true, pathRecur) // expect no folder created
	_, err = fmkdir.Apply(convertToFunctionArgs([]string{"-p", pathRecur}))
	assert.NoError(t, err)
	expect(false, pathRecur)
	// create multiple input
	_, err = fmkdir.Apply(convertToFunctionArgs(paths))
	assert.NoError(t, err)
	expect(false, paths[1:]...)
	// rmdir
	_, err = frmdir.Apply(convertToFunctionArgs([]string{path}))
	assert.Error(t, err)
	expect(false, path)
	_, err = frmdir.Apply(convertToFunctionArgs([]string{pathRecur}))
	assert.NoError(t, err)
	expect(true, pathRecur)
	p2dir := filepath.Dir(pathRecur)
	_, err = frmdir.Apply(convertToFunctionArgs([]string{"-p", p2dir}))
	assert.NoError(t, err)
	expect(true, path)
	expect(true, p2dir)
	_, err = frmdir.Apply(convertToFunctionArgs(paths))
	assert.NoError(t, err)
	expect(true, paths[1:]...)
}

func TestRm(t *testing.T) {
	a, b, c := "test", filepath.Join("test", "test1"), filepath.Join("test", "test2")
	fa, f1b, f2b, fc := filepath.Join(a, "a.txt"), filepath.Join(b, "b1.txt"), filepath.Join(b, "b2.txt"), filepath.Join(c, "c.txt")
	defer os.RemoveAll(a)
	err := os.MkdirAll(b, 0700)
	assert.NoError(t, err)
	err = os.MkdirAll(c, 0700)
	assert.NoError(t, err)
	err = os.WriteFile(fa, []byte("1atxt"), 0700)
	assert.NoError(t, err)
	err = os.WriteFile(f1b, []byte("fb1txt"), 0700)
	assert.NoError(t, err)
	err = os.WriteFile(f2b, []byte("fb2txt"), 0700)
	assert.NoError(t, err)
	err = os.WriteFile(fc, []byte("fctxt"), 0700)
	assert.NoError(t, err)
	f := GetFunction("rm")
	_, err = f.Apply(convertToFunctionArgs([]string{f1b}))
	assert.NoError(t, err)
	_, err = os.Stat(f1b)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = f.Apply(convertToFunctionArgs([]string{a}))
	assert.Error(t, err)
	_, err = f.Apply(convertToFunctionArgs([]string{"-r", a}))
	assert.NoError(t, err)
	for _, path := range []string{a, b, c, fa, f2b, fc} {
		_, err = os.Stat(path)
		assert.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestChangeDir(t *testing.T) {
	cdir, err := os.Getwd()
	require.NoError(t, err)
	tdir := filepath.Join("test", "abc")
	defer func() {
		os.Chdir(cdir)
		os.RemoveAll(filepath.Join(cdir, filepath.Dir(tdir)))
	}()
	wf := GetFunction("workin")
	cf := GetFunction("chdir")
	assert.Equal(t, wf, cf)
	assert.NoError(t, err)
	assert.NoError(t, os.MkdirAll(tdir, 0700))
	_, err = wf.Apply(convertToFunctionArgs([]string{tdir}))
	assert.NoError(t, err)
	ndir, err := os.Getwd()
	assert.NoError(t, err)
	assert.NotEqual(t, cdir, ndir)
	assert.Equal(t, ndir, filepath.Join(cdir, tdir))
	_, err = wf.Apply([]*args.FunctionArg{})
	require.NoError(t, err)
	ndir, err = os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, cdir, ndir)
}

func TestChown(t *testing.T) {
	u, g, f := "_**_nouser", "_**_nogroup", "file__not__exist"
	defer os.Remove(f)
	fn := GetFunction("chown")
	_, err := fn.Apply(convertToFunctionArgs([]string{u, f}))
	assert.Error(t, err)
	_, err = fn.Apply(convertToFunctionArgs([]string{u + ":" + g, f}))
	assert.Error(t, err)
	_, err = fn.Apply(convertToFunctionArgs([]string{":" + g, f}))
	assert.Error(t, err)
	// verify if chown success
	uid, gid := fmt.Sprintf("%d", os.Getuid()), fmt.Sprintf("%d", os.Getgid())
	if runtime.GOOS == "windows" {
		// should use GetUserName from advapi32.dll
		// or windows.GetUserNameEx
		user, err := osu.Lookup(os.Getenv("USERNAME"))
		require.NoError(t, err)
		uid, gid = user.Uid, user.Gid

	}
	assert.NoError(t, os.WriteFile(f, []byte("sample text"), 0700))
	_, err = fn.Apply(convertToFunctionArgs([]string{"-n", uid + ":" + gid, f}))
	assert.NoError(t, err)
	_, err = fn.Apply(convertToFunctionArgs([]string{"-n", uid, f}))
	assert.NoError(t, err)
	_, err = fn.Apply(convertToFunctionArgs([]string{"-n", ":" + gid, f}))
	assert.NoError(t, err)
	// user and group name
	user, err := osu.LookupId(uid)
	assert.NoError(t, err)
	group, err := osu.LookupGroupId(gid)
	assert.NoError(t, err)
	_, err = fn.Apply(convertToFunctionArgs([]string{"-n", user.Username + ":" + group.Name, f}))
	assert.Error(t, err)
	_, err = fn.Apply(convertToFunctionArgs([]string{user.Username + ":" + group.Name, f}))
	assert.NoError(t, err)
}

func TestChmod(t *testing.T) {
	a, b, c := "test", "test2", "test3"
	defer func() {
		os.Remove(a)
		os.Remove(b)
		os.Remove(c)
	}()
	expect := func(path, perm string) {
		mode, err := GetFDModePerm(path)
		assert.NoError(t, err)
		assert.Equal(t, perm, mode.String(), "file: %s", path)
	}
	assert.NoError(t, os.Mkdir(a, 0700))
	assert.NoError(t, os.Mkdir(b, 0710))
	assert.NoError(t, os.Mkdir(c, 0730))
	// os.Mkdir will ignore permission on window
	if runtime.GOOS == "windows" {
		Chmod(a, "0700")
		Chmod(b, "0710")
		Chmod(c, "0730")
	}
	// check permission
	expect(a, "-rwx------")
	expect(b, "-rwx--x---")
	// on macos go version go1.17.2 darwin/amd64, mkdir permission 0730 produce 0710 instead
	// it is probably os specific problem as go relied on syscall
	os.Chmod(c, 0730)
	expect(c, "-rwx-wx---")
	f := GetFunction("chmod")
	_, err := f.Apply(convertToFunctionArgs([]string{"go+x", a}))
	assert.NoError(t, err)
	expect(a, "-rwx--x--x")
	expect(b, "-rwx--x---")
	expect(c, "-rwx-wx---")
	_, err = f.Apply(convertToFunctionArgs([]string{"u-x,g-wx,o+r", b, c}))
	assert.NoError(t, err)
	expect(a, "-rwx--x--x")
	expect(b, "-rw----r--")
	expect(c, "-rw----r--")
}

func TestCopy(t *testing.T) {
	data1, data2, data3 := "sample text 1", "sample text 2", "sample text 3"
	dtop := "cp1"
	d, d2 := filepath.Join(dtop, "cp2", "cp3"), filepath.Join(dtop, "cp4")
	fcp1, fcp2, fcp3 := filepath.Join(dtop, "a.txt"), filepath.Join(dtop, "b.txt"), filepath.Join(dtop, "c.txt")
	fcp31, fcp32, fcp33 := filepath.Join(d, "a.txt"), filepath.Join(d, "b.txt"), filepath.Join(d, "c.txt")
	defer os.RemoveAll(dtop)
	assert.NoError(t, os.Mkdir(dtop, 0700))
	assert.NoError(t, os.MkdirAll(d, 0700))
	assert.NoError(t, os.WriteFile(fcp1, []byte(data1), 0700))
	assert.NoError(t, os.WriteFile(fcp2, []byte(data2), 0700))
	assert.NoError(t, os.WriteFile(fcp3, []byte(data3), 0700))
	// check alias was done properly
	fn := GetFunction("cp")
	fn1 := GetFunction("copy")
	assert.Equal(t, fn, fn1)
	expectContent := func(a, content string) {
		assert.FileExists(t, a)
		data, err := os.ReadFile(a)
		assert.NoError(t, err)
		assert.Equal(t, content, string(data))
	}
	// test copy file
	_, err := fn.Apply(convertToFunctionArgs([]string{fcp1, fcp31}))
	assert.NoError(t, err)
	expectContent(fcp31, data1)
	_, err = fn.Apply(convertToFunctionArgs([]string{fcp1, fcp31}))
	assert.NoError(t, err)
	_, err = fn.Apply(convertToFunctionArgs([]string{fcp2, fcp3, d}))
	assert.NoError(t, err)
	assert.FileExists(t, fcp32)
	assert.FileExists(t, fcp33)
	// test copy folder
	_, err = fn.Apply(convertToFunctionArgs([]string{d, d2}))
	assert.Error(t, err)
	_, err = fn.Apply(convertToFunctionArgs([]string{"-r", d, d2}))
	assert.NoError(t, err)
	assert.DirExists(t, d2)
	expectContent(filepath.Join(d2, filepath.Base(fcp31)), data1)
	expectContent(filepath.Join(d2, filepath.Base(fcp32)), data2)
	expectContent(filepath.Join(d2, filepath.Base(fcp33)), data3)
	_, err = fn.Apply(convertToFunctionArgs([]string{"-r", d + "/", d2}))
	assert.NoError(t, err)
	expectContent(filepath.Join(d2, filepath.Base(fcp31)), data1)
	expectContent(filepath.Join(d2, filepath.Base(fcp32)), data2)
	expectContent(filepath.Join(d2, filepath.Base(fcp33)), data3)
}

func TestMoveRename(t *testing.T) {
	dt1, dt2 := "sample1", "sample2"
	d1, d2, d3 := filepath.Join("test", "d1"), filepath.Join("test", "d2"), filepath.Join("test", "d3")
	f1, f2, f3 := filepath.Join(d3, "a.txt"), filepath.Join(d2, "b.txt"), filepath.Join(d2, "a.txt")
	fg1, f4 := filepath.Join(d2, "*.txt"), filepath.Join(d3, "b.txt")
	defer os.RemoveAll("test")
	assert.NoError(t, os.MkdirAll(d1, 0700))
	assert.NoError(t, os.MkdirAll(d2, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(d1, filepath.Base(f1)), []byte(dt1), 0700))
	assert.NoError(t, os.WriteFile(f2, []byte(dt2), 0700))
	fn := GetFunction("mv")
	fn1 := GetFunction("move")
	assert.Equal(t, fn, fn1)
	assert.DirExists(t, d1)
	assert.NoDirExists(t, d3)
	_, err := fn.Apply(convertToFunctionArgs([]string{d1, d3}))
	assert.NoError(t, err)
	assert.NoDirExists(t, d1)
	assert.DirExists(t, d3)
	assert.FileExists(t, f1)
	assert.NoFileExists(t, f3)
	_, err = fn.Apply(convertToFunctionArgs([]string{f1, f3}))
	assert.NoError(t, err)
	assert.NoFileExists(t, f1)
	assert.FileExists(t, f3)
	_, err = fn.Apply(convertToFunctionArgs([]string{fg1, d3}))
	assert.NoError(t, err)
	assert.NoFileExists(t, f2)
	assert.NoFileExists(t, f3)
	assert.FileExists(t, f1)
	assert.FileExists(t, f4)
	_, err = fn.Apply(convertToFunctionArgs([]string{f1, f4, d1}))
	assert.Error(t, err)
	_, err = fn.Apply(convertToFunctionArgs([]string{f1, f4, d2}))
	assert.NoError(t, err)
	assert.NoFileExists(t, f1)
	assert.NoFileExists(t, f4)
	assert.FileExists(t, f2)
	assert.FileExists(t, f3)
}
