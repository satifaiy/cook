package function

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	cookErrors "github.com/cozees/cook/pkg/errors"
	"github.com/cozees/cook/pkg/runtime/args"
)

func AllCXAFlags() []*args.Flags {
	return []*args.Flags{compressFlags, extractFlags}
}

type stateEntry struct {
	stat os.FileInfo
}

func (se *stateEntry) Name() string               { return se.stat.Name() }
func (se *stateEntry) IsDir() bool                { return se.stat.IsDir() }
func (se *stateEntry) Type() os.FileMode          { return se.stat.Mode() }
func (se *stateEntry) Info() (os.FileInfo, error) { return se.stat, nil }

func handleClose(closable io.Closer, err error) error {
	cerr := closable.Close()
	if err == nil {
		return cerr
	} else if cerr != nil {
		ckerr := &cookErrors.CookError{}
		ckerr.StackError(err)
		ckerr.StackError(cerr)
		return ckerr
	}
	return err
}

type compressOptions struct {
	Tar      bool   `flag:"tar"`
	Kind     string `flag:"kind"`
	Out      string `flag:"out"`
	Override bool   `flag:"override"`
	Mode     string `flag:"mode"`
	Verbose  bool   `flag:"verbose"`
	Args     []string

	// internal state
	verboseIO io.Writer
	ext       string
	needExt   bool
	mode      os.FileMode
	handler   func(w io.WriteCloser, opts *compressOptions) (any, error)
}

func (co *compressOptions) validate() error {
	if len(co.Args) == 0 {
		return errors.New("compress no input")
	} else if len(co.Args) > 1 {
		return errors.New("compress has to many input")
	}

	co.needExt = false
	m, err := filepath.Glob(co.Args[0])
	if co.Out == "" {
		if err == nil && (len(m) >= 1 && m[0] != co.Args[0]) {
			return errors.New("file name is require when input is a glob pattern")
		}
		co.Out = co.Args[0]
		if co.Out == "." || co.Out == ".." || co.Out == "./" || co.Out == "../" {
			if absOut, err := filepath.Abs(co.Out); err != nil {
				return err
			} else {
				co.Out = filepath.Base(absOut)
			}
		}
		co.needExt = true
	}
	istat, serr := os.Stat(co.Args[0])
	if err != nil && serr != nil {
		return fmt.Errorf("input %s is not exist", co.Args[0])
	}

	co.mode = 0777
	if co.Mode != "" {
		if co.mode, err = fm.Parse(0, co.Mode); err != nil {
			return err
		}
	}

	if co.Tar {
		co.ext = ".tar"
		if co.Kind == "" {
			co.handler = tarFileDir
			return nil
		}
	}

	if co.Verbose {
		co.verboseIO = os.Stdout
	}

	switch co.Kind {
	case "gzip":
		if !co.Tar && ((err == nil && (len(m) >= 1 && m[0] != co.Args[0])) || istat.IsDir()) {
			return errors.New("gzip cannnot be use to compress a folder or multiple file/folder, it must use with tarball")
		}
		co.ext += ".gz"
		co.handler = gzipFileDir
	case "zip":
		if co.Tar {
			return errors.New("zip not unsupported to combine with tarball (tar)")
		}
		co.ext = ".zip"
		co.handler = zipFileDir
	default:
		return fmt.Errorf("unsupported compression type %s", co.Kind)
	}
	return nil
}

const (
	compressorDesc = `The Compress function compress the file or directory.
					  It supported format 7z(lzma), xz, zip, gzip, bzip2, tar, rar.`
	kindDesc        = `Providing compressor the algorithms to compress the data. By default gzip is used.`
	tarDesc         = `Tell compressor to output as tar file`
	modeDesc        = `providing a unix like permission to apply to the output file. By default, the permission is set to 0777.`
	overrideDesc    = `Tell compressor to override the output file if its exist`
	verboseDesc     = `Tell compressor to display each compressed file or folder`
	compressOutDesc = `Tell compressor where to produce the output result. It is
					   file name or path to the output file.`
)

var compressFlags = &args.Flags{
	Flags: []*args.Flag{
		{Short: "k", Long: "kind", Description: kindDesc},
		{Short: "o", Long: "out", Description: compressOutDesc},
		{Short: "t", Long: "tar", Description: tarDesc},
		{Short: "f", Long: "override", Description: overrideDesc},
		{Short: "m", Long: "mode", Description: modeDesc},
		{Short: "v", Long: "verbose", Description: verboseDesc},
	},
	Result:      reflect.TypeOf((*compressOptions)(nil)).Elem(),
	FuncName:    "compress",
	Example:     "@compress -a gzip --tar folder",
	ShortDesc:   "Compress/Archive folder or file.",
	Usage:       "@compress [-v] [-m 0700] [-f] [--tar] [-o DIRECTORY|FILE] [-k algo] FILE",
	Description: compressorDesc,
}

type listFileDirFunc func(source, path string, d fs.DirEntry, err error) error

func rootDir(files []string) string {
	last, d, sp := 0, "", fmt.Sprintf("%c", os.PathSeparator)
	for _, f := range files {
		fd := strings.Count(f, sp)
		if d == "" {
			d = filepath.Dir(f)
			last = fd
		} else if fd < last {
			last = fd
			d = filepath.Dir(f)
		}
	}
	return filepath.Dir(d)
}

func listFileDir(opts *compressOptions, fn listFileDirFunc) error {
	walkHandler := func(source, s string) error {
		stat, err := os.Stat(s)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			return filepath.WalkDir(s, func(path string, d fs.DirEntry, err error) error { return fn(s, path, d, err) })
		} else {
			return fn(s, s, &stateEntry{stat: stat}, nil)
		}
	}
	gfiles, err := filepath.Glob(opts.Args[0])
	if gfiles == nil || err != nil {
		// not a glob pattern
		if err = walkHandler(filepath.Dir(opts.Args[0]), opts.Args[0]); err != nil {
			return err
		}
	} else {
		source := rootDir(gfiles)
		for _, file := range gfiles {
			if err = walkHandler(source, file); err != nil {
				return err
			}
		}
	}
	return nil
}

func logTarVerbose(w io.Writer, kind string, args ...any) {
	if kind != "" {
		kind = "tar." + kind
	} else {
		kind = "tar"
	}
	args = append(args[:2], args[1:]...)
	args[1] = kind
	fmt.Fprintf(w, "%s (%s) %s: %s\n", args...)
}

func tarFileDir(w io.WriteCloser, opts *compressOptions) (v any, err error) {
	tw := tar.NewWriter(w)
	defer func() { err = handleClose(tw, err) }()
	return nil, listFileDir(opts, func(source, path string, d fs.DirEntry, err error) error {
		stat, err := GetFDStat(path)
		if err != nil {
			return err
		}
		header := &tar.Header{
			Name:    path,
			Mode:    int64(stat.Mode()),
			Size:    stat.Size(),
			ModTime: stat.ModTime(),
		}

		if d.IsDir() {
			if opts.verboseIO != nil {
				logTarVerbose(opts.verboseIO, opts.Kind, "archive", "folder", path)
			}
			header.Typeflag = tar.TypeDir
			return tw.WriteHeader(header)
		} else {
			header.Typeflag = tar.TypeReg
			if err = tw.WriteHeader(header); err != nil {
				return err
			} else if file, err := os.Open(path); err != nil {
				return err
			} else {
				if opts.verboseIO != nil {
					logTarVerbose(opts.verboseIO, opts.Kind, "archive", "file", path)
				}
				defer file.Close()
				_, err = io.Copy(tw, file)
				return err
			}
		}
	})
}

func gzipFileDir(w io.WriteCloser, opts *compressOptions) (v any, err error) {
	gw := gzip.NewWriter(w)
	defer func() { err = handleClose(gw, err) }()
	if opts.Tar {
		return tarFileDir(gw, opts)
	} else {
		// validateCompress already ensure that the input is a single input argument and it's not
		// a folder nor a glob pattern.
		return nil, listFileDir(opts, func(source, path string, d fs.DirEntry, err error) (rerr error) {
			if err == nil {
				gw.Name = path
				if fi, err := d.Info(); err != nil {
					return err
				} else {
					gw.ModTime = fi.ModTime()
				}
				var f *os.File
				if f, err = os.Open(path); err != nil {
					return err
				} else {
					if opts.verboseIO != nil {
						fmt.Fprintf(opts.verboseIO, "   gzip file: %s\n", path)
					}
					defer func() { rerr = handleClose(f, rerr) }()
					_, err = io.Copy(gw, f)
					return err
				}
			}
			return err
		})
	}
}

func zipFileDir(w io.WriteCloser, opts *compressOptions) (v any, err error) {
	zw := zip.NewWriter(w)
	defer func() { err = handleClose(zw, err) }()
	return nil, listFileDir(opts, func(source, path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info, err := d.Info(); err != nil {
			return err
		} else if header, err := zip.FileInfoHeader(info); err != nil {
			return err
		} else {
			header.Method = zip.Deflate
			if source == path {
				header.Name = path
			} else if header.Name, err = filepath.Rel(source, path); err != nil {
				return err
			}

			if info.IsDir() {
				header.Name += "/"
				// create header for folder return dummy header writer therefore we ignore it
				_, err = zw.CreateHeader(header)
				return err
			}

			if hw, err := zw.CreateHeader(header); err != nil {
				return err
			} else if f, err := os.Open(path); err != nil {
				return err
			} else {
				if opts.verboseIO != nil {
					fmt.Fprintf(opts.verboseIO, "   zip file: %s\n", path)
				}
				defer f.Close()
				_, err = io.Copy(hw, f)
				return err
			}
		}
	})
}

var compressFn = NewBaseFunction(compressFlags, func(f Function, i any) (v any, err error) {
	opts := i.(*compressOptions)
	if err = opts.validate(); err != nil {
		return nil, err
	}
	// open file
	filename := opts.Out
	if opts.needExt {
		filename += opts.ext
	}
	flags := os.O_WRONLY | os.O_CREATE
	stat, err := os.Stat(filename)
	if err == nil && stat != nil {
		if !opts.Override {
			return nil, fmt.Errorf("output file %s is already existed", filename)
		}
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(filename, flags, opts.mode)
	if err != nil {
		return nil, err
	}
	defer func() { err = file.Close() }()
	return opts.handler(file, opts)
})

type extractOptions struct {
	Out     string `flag:"out"`
	Mode    string `flag:"mode"`
	Verbose bool   `flag:"verbose"`
	Args    []string

	// use internal for writing verbose output
	mode      os.FileMode
	verboseIO io.Writer
}

const (
	extractorDesc = `The extractor function extract the file or directory from the compressed file.
					 It support format 7z(lzma), xz, zip, gzip, bzip2, tar, rar.`
	extractOutDesc = `Tell extractor where to extract file and/or folder to. If folder is not exist
					  extractor will create it.`
	verboseXDesc = `Tell extractor to display each extracted file or folder`
	modeXDesc    = `override/provide permission to all file or folder extracted from compress/archive file.
				   By default, it apply the permission based on the permission available in the archive/compressed file
				   however if there is no permisson available then 0777 permission is used.`
)

var extractFlags = &args.Flags{
	Flags: []*args.Flag{
		{Short: "o", Long: "out", Description: extractOutDesc},
		{Short: "m", Long: "mode", Description: modeXDesc},
		{Short: "v", Long: "verbose", Description: verboseXDesc},
	},
	Result:      reflect.TypeOf((*extractOptions)(nil)).Elem(),
	FuncName:    "extract",
	Example:     "@extract sample.tar.gz",
	ShortDesc:   "Decompress the data.",
	Usage:       "@extract [-v] [-m 0700] [-o DIRECTORY|FILE] FILE",
	Description: extractorDesc,
}

func extractZipFile(reader io.ReaderAt, size int64, opts *extractOptions) (err error) {
	zr, err := zip.NewReader(reader, size)
	if err != nil {
		return err
	}
	for _, zf := range zr.File {
		dest := filepath.Join(opts.Out, zf.Name)
		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(dest, filepath.Clean(opts.Out)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", zf.Name)
		}

		mode := zf.Mode()
		if opts.mode != 0777 {
			mode = opts.mode
		}

		if zf.FileInfo().IsDir() {
			// Make Folder
			if err = os.MkdirAll(dest, mode); err != nil {
				if opts.verboseIO != nil {
					fmt.Fprintf(opts.verboseIO, "   extract folder: %s\n", dest)
				}
				return err
			}
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(dest), mode); err != nil {
			return err
		}

		of, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return err
		}

		rc, err := zf.Open()
		if err != nil {
			of.Close()
			return err
		}

		if opts.verboseIO != nil {
			fmt.Fprintf(opts.verboseIO, "   extract file: %s\n", dest)
		}
		_, err = io.Copy(of, rc)

		of.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

func extractGzipFile(r io.ReadSeeker, opts *extractOptions) (err error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()
	buf := make([]byte, 512)
	if n, err := gr.Read(buf); err != nil {
		return err
	} else {
		// the content is tarbal file extract tar
		if _, err = r.Seek(0, 0); err != nil {
			return err
		} else if tt := FileDataType(buf[:n]); tt == TarFile {
			gr.Reset(r)
			return extractTarFile(gr, opts)
		}
		gr.Reset(r)
		// continue to extract content of gzip.
		dest := filepath.Join(opts.Out, gr.Name)
		dir := filepath.Dir(dest)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err = os.MkdirAll(dir, opts.mode); err != nil {
				return err
			}
		}
		// given no executing permission by default
		var of *os.File
		of, err = os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, opts.mode)
		if err != nil {
			return err
		}
		if opts.verboseIO != nil {
			fmt.Fprintf(opts.verboseIO, "   extract file: %s\n", dest)
		}
		defer func() { err = handleClose(of, err) }()
		_, err = io.Copy(of, gr)
		return err
	}
}

func extractTarFile(r io.Reader, opts *extractOptions) (err error) {
	tr := tar.NewReader(r)
	var header *tar.Header
	var of *os.File
	for header, err = tr.Next(); err == nil; header, err = tr.Next() {
		switch header.Typeflag {
		case tar.TypeDir:
			dirOut := filepath.Join(opts.Out, header.Name)
			if err = os.MkdirAll(dirOut, opts.mode); err != nil {
				return err
			}
			if opts.verboseIO != nil {
				fmt.Fprintf(opts.verboseIO, "   extract folder: %s\n", dirOut)
			}
		case tar.TypeReg:
			fout := filepath.Join(opts.Out, header.Name)
			if of, err = os.OpenFile(fout, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, opts.mode); err != nil {
				return err
			}
			if opts.verboseIO != nil {
				fmt.Fprintf(opts.verboseIO, "   extract file: %s\n", fout)
			}
			if _, err := io.Copy(of, tr); err != nil {
				of.Close()
				return err
			}
			of.Close()
		default:
			return fmt.Errorf("extract tar: uknown type: %c in %s", header.Typeflag, header.Name)
		}
	}
	if err == io.EOF {
		return nil
	}
	return err
}

func extractHandler(buf []byte, file string, opts *extractOptions) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := f.Read(buf)
	if err != nil {
		return err
	}
	// reset to beginning
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	ft := FileDataType(buf[:n])
	switch ft {
	case ZipFile:
		return extractZipFile(f, fi.Size(), opts)
	case TarFile:
		return extractTarFile(f, opts)
	case GzipFile:
		return extractGzipFile(f, opts)
	default:
		return fmt.Errorf("unsupport type %s archive/compress file of %s", ft, file)
	}
}

var extractFn = NewBaseFunction(extractFlags, func(f Function, i any) (any, error) {
	opts := i.(*extractOptions)
	if opts.Verbose {
		opts.verboseIO = os.Stdout
	}
	var err error
	if opts.Out == "" {
		if opts.Out, err = os.Getwd(); err != nil {
			return nil, err
		}
	}

	opts.mode = 0777
	if opts.Mode != "" {
		if opts.mode, err = fm.Parse(0, opts.Mode); err != nil {
			return nil, err
		}
	}

	if _, err = os.Stat(opts.Out); os.IsNotExist(err) {
		if err = os.MkdirAll(opts.Out, opts.mode); err != nil {
			return nil, err
		}
	}

	if len(opts.Args) == 0 {
		return nil, errors.New("no file to extract")
	}
	buf := make([]byte, 512)
	for _, file := range opts.Args {
		if err = extractHandler(buf, file, opts); err != nil {
			return nil, err
		}
	}
	return nil, nil
})

func init() {
	registerFunction(compressFn)
	registerFunction(extractFn)
}
