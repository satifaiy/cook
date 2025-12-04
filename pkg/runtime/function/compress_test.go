package function

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cozees/cook/pkg/runtime/args"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func verifyFileContent(t *testing.T, f1, f2 string) {
	cf1, err := os.ReadFile(f1)
	require.NoError(t, err)
	cf2, err := os.ReadFile(f2)
	require.NoError(t, err)
	assert.Equal(t, string(cf1), string(cf2))
}

var extractInput = []string{
	filepath.Join("testdata", "sample.txt.gz"),
	filepath.Join("testdata", "sample.txt.tar"),
	filepath.Join("testdata", "sample.txt.tar.gz"),
	filepath.Join("testdata", "sample.txt.zip"),
}

func TestExtractFile(t *testing.T) {
	fn := GetFunction("extract")
	outfile := "sample.txt"
	source := filepath.Join("testdata", "sample.txt")
	defer os.Remove(outfile)
	for _, tc := range extractInput {
		t.Logf("TestExtractFile test extract %s file", tc)
		_, err := fn.Apply([]*args.FunctionArg{
			{Val: tc, Kind: reflect.String},
		})
		require.NoError(t, err)
		verifyFileContent(t, source, outfile)
		os.Remove(outfile)
	}
}

var compressInput = []string{
	"gzip",
	"tar",
	"tar,gzip",
	"zip",
}

func compressInputArgument(tc, source, out string) []*args.FunctionArg {
	pargs := make([]*args.FunctionArg, 0)
	if strings.HasPrefix(tc, "tar") {
		pargs = append(pargs, &args.FunctionArg{Val: "-t", Kind: reflect.String})
		if strings.HasSuffix(tc, ",gzip") {
			pargs = append(pargs, &args.FunctionArg{Val: "-k", Kind: reflect.String})
			pargs = append(pargs, &args.FunctionArg{Val: "gzip", Kind: reflect.String})
		}
	} else {
		pargs = append(pargs, &args.FunctionArg{Val: "-k", Kind: reflect.String})
		pargs = append(pargs, &args.FunctionArg{Val: tc, Kind: reflect.String})
	}
	pargs = append(pargs, &args.FunctionArg{Val: "-o", Kind: reflect.String})
	pargs = append(pargs, &args.FunctionArg{Val: out, Kind: reflect.String})
	pargs = append(pargs, &args.FunctionArg{Val: source, Kind: reflect.String})
	return pargs
}

func TestCompressFile(t *testing.T) {
	fn := GetFunction("compress")
	xfn := GetFunction("extract")
	smcp := "sample.compress"
	xsmcpdir := ".compress.out"
	source := filepath.Join("testdata", "sample.txt")
	xsmcp := filepath.Join(xsmcpdir, source)
	defer os.Remove(smcp)
	defer os.RemoveAll(xsmcpdir)
	for _, tc := range compressInput {
		t.Logf("TestCompressFile test compress %s kind", tc)
		_, err := fn.Apply(compressInputArgument(tc, source, smcp))
		require.NoError(t, err)
		assert.FileExists(t, smcp)
		_, err = xfn.Apply([]*args.FunctionArg{
			{Val: "-o", Kind: reflect.String},
			{Val: xsmcpdir, Kind: reflect.String},
			{Val: smcp, Kind: reflect.String},
		})
		require.NoError(t, err)
		assert.FileExists(t, xsmcp)
		verifyFileContent(t, source, xsmcp)
		os.Remove(xsmcp)
		os.Remove(smcp)
	}
}

var extractDirTestCase = []string{
	filepath.Join("testdata", "dir.tar"),
	filepath.Join("testdata", "dir.tar.gz"),
	filepath.Join("testdata", "dir.zip"),
}

func TestCompressExtractDir(t *testing.T) {
	fn := GetFunction("compress")
	xfn := GetFunction("extract")
	outdir := ".compressdir"
	sourceFile := filepath.Join("testdata", "sample.txt")
	defer os.RemoveAll(outdir)
	// test extract dir
	for _, file := range extractDirTestCase {
		t.Logf("TestCompressExtractDir test extract %s file", file)
		_, err := xfn.Apply([]*args.FunctionArg{
			{Val: "-o", Kind: reflect.String},
			{Val: outdir, Kind: reflect.String},
			{Val: file, Kind: reflect.String},
		})
		require.NoError(t, err)
		assert.DirExists(t, filepath.Join(outdir, "dir"))
		assert.DirExists(t, filepath.Join(outdir, "dir", "dir1"))
		assert.FileExists(t, filepath.Join(outdir, "dir", "dir1", "a.txt"))
		assert.DirExists(t, filepath.Join(outdir, "dir", "dir2"))
		assert.FileExists(t, filepath.Join(outdir, "dir", "dir2", "b.txt"))
		verifyFileContent(t, sourceFile, filepath.Join(outdir, "dir", "dir1", "a.txt"))
		verifyFileContent(t, sourceFile, filepath.Join(outdir, "dir", "dir2", "b.txt"))
		os.RemoveAll(outdir)
	}
	// test compress dir
	outcompress := "dir.compress"
	source := filepath.Join("testdata", "dir")
	defer os.Remove(outcompress)
	for _, tc := range compressInput {
		t.Logf("TestCompressExtractDir test compress kind %s", tc)
		_, err := fn.Apply(compressInputArgument(tc, source, outcompress))
		if tc == "gzip" {
			assert.Error(t, err)
			continue
		} else {
			require.NoError(t, err)
			require.FileExists(t, outcompress)
		}
		// try extract
		_, err = xfn.Apply([]*args.FunctionArg{
			{Val: "-o", Kind: reflect.String},
			{Val: outdir, Kind: reflect.String},
			{Val: outcompress, Kind: reflect.String},
		})
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(outdir, source, "dir1", "a.txt"))
		require.FileExists(t, filepath.Join(outdir, source, "dir2", "b.txt"))
		verifyFileContent(t, sourceFile, filepath.Join(outdir, source, "dir1", "a.txt"))
		verifyFileContent(t, sourceFile, filepath.Join(outdir, source, "dir2", "b.txt"))
		os.RemoveAll(outcompress)
	}
}
