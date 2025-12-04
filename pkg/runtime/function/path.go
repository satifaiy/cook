package function

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cozees/cook/pkg/runtime/args"
)

func AllPathFlags() []*args.Flags {
	return []*args.Flags{pbaseFlags, pabsFlags, pcleanFlags, pdirFlags, pextFlags, psplitFlags, prelFlags, pglobFlags}
}

type pathOptions struct {
	Args []string
}

func validate(f Function, opts *pathOptions, narg int, fn func(...string) (any, error)) (any, error) {
	if len(opts.Args) != narg {
		return "", fmt.Errorf("%s require %d arugment file path", f.Name(), narg)
	}
	return fn(opts.Args...)
}

func dHandler(f Function, i any, narg int, fn func(string) (string, error)) (any, error) {
	return validate(f, i.(*pathOptions), narg, func(s ...string) (any, error) { return fn(s[0]) })
}

func sHandler(f Function, i any, narg int, fn func(string) string) (any, error) {
	return validate(f, i.(*pathOptions), narg, func(s ...string) (any, error) { return fn(s[0]), nil })
}

var pathOptsType = reflect.TypeOf((*pathOptions)(nil)).Elem()

var pabsFlags = &args.Flags{
	Result:    pathOptsType,
	FuncName:  "pabs",
	ShortDesc: "convert relative path to an absolute path",
	Usage:     "@pabs FILEPATH",
	Example:   "@pabs dir/file.txt",
	Description: `Returns an absolute representation of path. If the path is not absolute it will be joined
				  with the current working directory to turn it into an absolute path. The absolute path name
				  for a given file is not guaranteed to be unique. The path is also being clean as well.`,
}

var pbaseFlags = &args.Flags{
	Result:    pathOptsType,
	FuncName:  "pbase",
	ShortDesc: "return last element path.",
	Usage:     "@pbase FILEPATH",
	Example:   "@pbase dir/file.txt",
	Description: `Returns the last element of path. Trailing path separators are removed before extracting
				  the last element. If the path is empty, Base returns ".". If the path consists entirely
				  of separators, Base returns a single separator.`,
}

var pextFlags = &args.Flags{
	Result:    pathOptsType,
	FuncName:  "pext",
	ShortDesc: "return file extension.",
	Usage:     "@pext FILEPATH",
	Example:   "@pext dir/file.txt",
	Description: `Returns the file name extension used by path. The extension is the suffix beginning at
				  the final dot in the final element of path; it is empty if there is no dot.`,
}

var pdirFlags = &args.Flags{
	Result:    pathOptsType,
	FuncName:  "pdir",
	ShortDesc: "return parent directory.",
	Usage:     "@pdir FILEPATH",
	Example:   "@pdir dir/file.txt",
	Description: `Returns all but the last element of path, typically the path's directory. After dropping
				  the final element, Dir calls Clean on the path and trailing slashes are removed.
				  If the path is empty, Dir returns ".". If the path consists entirely of separators, Dir
				  returns a single separator. The returned path does not end in a separator unless it is the root directory`,
}

var pcleanFlags = &args.Flags{
	Result:      pathOptsType,
	FuncName:    "pclean",
	ShortDesc:   `return a path without "." or ".."`,
	Usage:       "@pclean FILEPATH",
	Example:     "@pclean ./dir/../file.txt",
	Description: `Returns the shortest path name without "." or ".."`,
}

var psplitFlags = &args.Flags{
	Result:      pathOptsType,
	FuncName:    "psplit",
	ShortDesc:   `return array string split by path separtor`,
	Usage:       "@psplit FILEPATH",
	Example:     "@psplit dir/file.txt",
	Description: `Returns array string of each segment in file path which separate by path separator.`,
}

var pglobFlags = &args.Flags{
	Result:    pathOptsType,
	FuncName:  "pglob",
	ShortDesc: `return array file path that match the pattern`,
	Usage:     "@pglob GLOB_PATTERN",
	Example:   "@pglob dir/*.txt",
	Description: `Returns the names of all files matching pattern or nil if there is no matching file.
				  The syntax of patterns is the same as in Match. The pattern may describe hierarchical
				  names such as /usr/*/bin/ed (assuming the Separator is '/').`,
}

var prelFlags = &args.Flags{
	Result:    pathOptsType,
	FuncName:  "prel",
	ShortDesc: `return path relative`,
	Usage:     "@prel REFERENCE_PATH TO_PATH",
	Example:   "@prel dir/a dir/sample/../a/file.txt",
	Description: `Returns a relative path that is lexically equivalent to targpath when joined to basepath
				  with an intervening separator. On success, the returned path will always be relative to
				  reference path, even if reference path and to path share no elements`,
}

func init() {
	registerFunction(NewBaseFunction(pabsFlags, func(f Function, i any) (any, error) {
		return dHandler(f, i, 1, filepath.Abs)
	}))

	registerFunction(NewBaseFunction(pbaseFlags, func(f Function, i any) (any, error) {
		return sHandler(f, i, 1, filepath.Base)
	}))

	registerFunction(NewBaseFunction(pextFlags, func(f Function, i any) (any, error) {
		return sHandler(f, i, 1, filepath.Ext)
	}))

	registerFunction(NewBaseFunction(pdirFlags, func(f Function, i any) (any, error) {
		return sHandler(f, i, 1, filepath.Dir)
	}))

	registerFunction(NewBaseFunction(pcleanFlags, func(f Function, i any) (any, error) {
		return sHandler(f, i, 1, filepath.Clean)
	}))

	registerFunction(NewBaseFunction(psplitFlags, func(f Function, i any) (any, error) {
		return validate(f, i.(*pathOptions), 1, func(s ...string) (any, error) {
			return strings.Split(s[0], fmt.Sprintf("%c", os.PathSeparator)), nil
		})
	}))

	registerFunction(NewBaseFunction(pglobFlags, func(f Function, i any) (any, error) {
		return validate(f, i.(*pathOptions), 1, func(s ...string) (any, error) {
			return filepath.Glob(s[0])
		})
	}))

	registerFunction(NewBaseFunction(prelFlags, func(f Function, i any) (any, error) {
		return validate(f, i.(*pathOptions), 2, func(s ...string) (any, error) {
			return filepath.Rel(s[0], s[1])
		})
	}))
}
