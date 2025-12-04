package function

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"

	"github.com/cozees/cook/pkg/runtime/args"
)

func AllHttpFlags() []*args.Flags {
	return []*args.Flags{getFlags, headFlags, optionsFlags, postFlags, putFlags, deleteFlags, patchFlags}
}

type httpOption struct {
	Header       http.Header `flag:"header"`
	File         string      `flag:"file"`
	Data         string      `flag:"data"`
	Restriction  bool        `flag:"strict"`
	IsMetionData bool        `mention:"data"` // true if argument flag data is given even the value is zero/empty string ""
	Args         []string
}

func (ho *httpOption) validate(name string) (string, error) {
	if len(ho.Args) != 1 {
		return "", fmt.Errorf("function %s required one last argument as URL", name)
	} else {
		return ho.Args[0], nil
	}
}

const (
	headerDesc = `custom http header to be include or override existing header in the request.`
	dataDesc   = `string data to be sent to the server. Although, by default the data is an empty string, function will not send
				  empty string to the server unless it was explicit in argument with --data "".`
	fileDesc = `a path to a file which it's content is being used as the data to send to the server.
				  Note: if both flag "file" and "data" is given at the same time then flag "file" is used instead of "data".`
	strictDesc = `enforce the http request and response to follow the standard of http definition for each method.`

	// Common introduction for any http function
	baseFnDesc = `Send an http request to the server at [URL] and return a response map which contain
			  	  two key "header" and "body". The "header" is a map of response header where the "body"
				  is a reader object if there is data in the body.`
	largeBodyDesc = `function can be use with redirect statement as well as assign statement.
			 		 However if the data from the function is too large it's better to use redirect
					 statement to store the data in a file instead.`
	noBodyRespDesc = `request should not have response body thus if the a restriction flag is given the
					  function will cause program to halt the execution otherwise a warning message is
					  written to standard output instead.`
)

var httpNoBodyFlags = []*args.Flag{
	{Short: "h", Long: "header", Description: headerDesc},
	{Long: "strict", Description: strictDesc},
}

var httpFlags = []*args.Flag{
	{Short: "h", Long: "header", Description: headerDesc},
	{Short: "d", Long: "data", Description: dataDesc},
	{Short: "f", Long: "file", Description: fileDesc},
	{Long: "strict", Description: strictDesc},
}

type readerCloser struct {
	*bytes.Reader
}

func (rc *readerCloser) Close() error { return nil }

func detectContentType(r io.ReadSeekCloser) string {
	defer r.Seek(0, 0)
	buf := [512]byte{}
	return http.DetectContentType(buf[:])
}

// for override testing purpose only
var returnFunc = func(resp *http.Response, canHasResponseBody bool) any {
	if canHasResponseBody {
		return resp.Body
	}
	return nil
}

func httpRequest(bf Function, i any, method string) (result any, err error) {
	opts := i.(*httpOption)
	url, err := opts.validate(bf.Name())
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	var req *http.Request
	var body io.ReadSeekCloser
	var canResponseHasBody = false
	switch method {
	case http.MethodOptions, http.MethodGet:
		canResponseHasBody = true
	case http.MethodDelete, http.MethodPost, http.MethodPatch:
		canResponseHasBody = true
		fallthrough
	case http.MethodPut:
		if opts.File != "" {
			body, err = os.Open(opts.File)
			if err != nil {
				return nil, err
			}
		} else if opts.IsMetionData {
			body = &readerCloser{Reader: bytes.NewReader([]byte(opts.Data))}
		}
	}

	if req, err = http.NewRequest(method, url, body); err != nil {
		return nil, err
	}
	// set header if available
	if opts.Header != nil {
		req.Header = http.Header(opts.Header)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", detectContentType(body))
	}
	if resp, err = http.DefaultClient.Do(req); err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		return returnFunc(resp, canResponseHasBody), nil
	case resp.StatusCode < 300:
		return nil, nil
	default:
		return nil, fmt.Errorf("server report %d on get %s", resp.StatusCode, url)
	}
}

var httpOptsType = reflect.TypeOf((*httpOption)(nil)).Elem()

var getFlags = &args.Flags{
	Flags:       httpNoBodyFlags,
	Result:      httpOptsType,
	FuncName:    "get",
	ShortDesc:   "send http get request",
	Usage:       "@get [-h key:val [-h key:value] ...] URL",
	Example:     "@get -h X-Sample:123 https://www.example.com",
	Description: baseFnDesc + " @get " + largeBodyDesc,
}

var headFlags = &args.Flags{
	Flags:       httpNoBodyFlags,
	Result:      httpOptsType,
	FuncName:    "head",
	ShortDesc:   "send http head request",
	Usage:       "@head [-h key:val [-h key:value] ...] URL",
	Example:     "@head -h X-Sample:123 https://www.example.com",
	Description: baseFnDesc + " Note: By standard, head " + noBodyRespDesc,
}

var optionsFlags = &args.Flags{
	Flags:       httpNoBodyFlags,
	Result:      httpOptsType,
	FuncName:    "options",
	ShortDesc:   "send http options request",
	Usage:       "@options [-h key:val [-h key:value] ...] URL",
	Example:     "@options -h X-Sample:123 https://www.example.com",
	Description: baseFnDesc + " @option " + largeBodyDesc,
}

var postFlags = &args.Flags{
	Flags:       httpFlags,
	Result:      httpOptsType,
	FuncName:    "post",
	ShortDesc:   "send http post request",
	Usage:       "@post [-h key:val [-h key:value] ...] [-d data] [-f file] URL",
	Example:     "@post -h Content-Type:application/json -d '{\"key\":123}' https://www.example.com",
	Description: baseFnDesc + " @post " + largeBodyDesc,
}

var patchFlags = &args.Flags{
	Flags:       httpFlags,
	Result:      httpOptsType,
	FuncName:    "patch",
	ShortDesc:   "send http patch request",
	Usage:       "@patch [-h key:val [-h key:value] ...] [-d data] [-f file] URL",
	Example:     "@patch -h Content-Type:application/json -d '{\"key\":123}' https://www.example.com",
	Description: baseFnDesc + " @patch " + largeBodyDesc,
}

var putFlags = &args.Flags{
	Flags:       httpFlags,
	Result:      httpOptsType,
	FuncName:    "put",
	ShortDesc:   "send http put request",
	Usage:       "@put [-h key:val [-h key:value] ...] [-d data] [-f file] URL",
	Example:     "@put -h Content-Type:application/json -d '{\"key\":123}' https://www.example.com",
	Description: baseFnDesc + " Note: By standard, put " + noBodyRespDesc,
}

var deleteFlags = &args.Flags{
	Flags:       httpFlags,
	Result:      httpOptsType,
	FuncName:    "delete",
	ShortDesc:   "send http delete request",
	Usage:       "@delete [-h key:val [-h key:value] ...] [-d data] [-f file] URL",
	Example:     "@delete -h Content-Type:application/json -d '{\"key\":123}' https://www.example.com",
	Description: baseFnDesc + " @delete " + largeBodyDesc,
}

func init() {
	registerFunction(NewBaseFunction(getFlags, func(f Function, i any) (any, error) {
		return httpRequest(f, i, http.MethodGet)
	}, "fetch"))

	registerFunction(NewBaseFunction(headFlags, func(f Function, i any) (any, error) {
		return httpRequest(f, i, http.MethodHead)
	}))

	registerFunction(NewBaseFunction(optionsFlags, func(f Function, i any) (any, error) {
		return httpRequest(f, i, http.MethodOptions)
	}))

	registerFunction(NewBaseFunction(postFlags, func(f Function, i any) (any, error) {
		return httpRequest(f, i, http.MethodPost)
	}))

	registerFunction(NewBaseFunction(patchFlags, func(f Function, i any) (any, error) {
		return httpRequest(f, i, http.MethodPatch)
	}))

	registerFunction(NewBaseFunction(putFlags, func(f Function, i any) (any, error) {
		return httpRequest(f, i, http.MethodPut)
	}))

	registerFunction(NewBaseFunction(deleteFlags, func(f Function, i any) (any, error) {
		return httpRequest(f, i, http.MethodDelete)
	}))
}
