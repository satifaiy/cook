package function

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/cozees/cook/pkg/runtime/args"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	returnFunc = func(resp *http.Response, _ bool) any { return resp }
}

func responseResponse(i any, method string) (string, error) {
	if i == nil {
		return "", fmt.Errorf("no response")
	}
	resp := i.(*http.Response)
	keys := []string{}
	for k := range resp.Header {
		if strings.HasPrefix(k, "R-") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	sb := bytes.NewBufferString("")
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(strings.Join(resp.Header[k], "; "))
		sb.WriteByte('\n')
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		if resp.Body != nil && resp.Body != http.NoBody {
			defer resp.Body.Close()
			if _, err := io.Copy(sb, resp.Body); err != nil {
				return "", err
			}
		}
	}
	return sb.String(), nil
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

const jsonFile = "sample"
const jsonContent = `{"mine": "memo", "kind":"cartoon"}`

func TestMain(t *testing.M) {
	if err := setupHttpTest(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(-1)
	}
	code := t.Run()
	cleanupHttpTest()
	os.Exit(code)
}

func setupHttpTest() error {
	return os.WriteFile(jsonFile, []byte(jsonContent), 0700)
}

func cleanupHttpTest() {
	os.Remove(jsonFile)
}

type httpTestInOut struct {
	args    []*args.FunctionArg
	methods []string
	err     bool
	output  any
}

var getTestCase = []httpTestInOut{
	{ // case 1
		args:    convertToFunctionArgs([]string{}),
		methods: []string{http.MethodGet, http.MethodOptions},
		output:  "R-Method: %s\nBody: TEXT",
	},
	{ // case 2
		args:    convertToFunctionArgs([]string{}),
		methods: []string{http.MethodHead},
		output:  "R-Method: %s\n",
	},
	{ // case 3
		args:    convertToFunctionArgs([]string{"-h", "X-SESSION-1:abc"}),
		methods: []string{http.MethodGet, http.MethodOptions},
		output:  "R-Method: %s\nR-X-Session-1: abc\nBody: TEXT",
	},
	{ // case 4
		args:    convertToFunctionArgs([]string{"-h", "X-SESSION-1:abc", "-h", "X-SESSION-1:123"}),
		methods: []string{http.MethodGet, http.MethodOptions},
		output:  "R-Method: %s\nR-X-Session-1: abc; 123\nBody: TEXT",
	},
	{ // case 5
		args:    convertToFunctionArgs([]string{"-h", "X-SESSION-1:abc", "-h", "X-SESSION-1:123", "-h", "X-SESSION-2:23.2"}),
		methods: []string{http.MethodGet, http.MethodOptions},
		output:  "R-Method: %s\nR-X-Session-1: abc; 123\nR-X-Session-2: 23.2\nBody: TEXT",
	},
	{ // case 6
		args:    convertToFunctionArgs([]string{"-h", "X-SESSION-1:abc", "-h", "X-SESSION-1:123", "-h", "X-SESSION-2:23.2"}),
		methods: []string{http.MethodHead},
		output:  "R-Method: %s\nR-X-Session-1: abc; 123\nR-X-Session-2: 23.2\n",
	},
	{ // case 7
		args:    convertToFunctionArgs([]string{"-d", "simple"}),
		methods: []string{http.MethodPost, http.MethodPatch, http.MethodDelete},
		output:  "R-Content-Type: application/octet-stream\nR-Method: %s\nsimple",
	},
	{ // case 8
		args:    convertToFunctionArgs([]string{"-h", "Content-Type: text/plain", "-d", "simple"}),
		methods: []string{http.MethodPost, http.MethodPatch, http.MethodDelete},
		output:  "R-Content-Type: text/plain\nR-Method: %s\nsimple",
	},
	{ // case 9
		args:    convertToFunctionArgs([]string{"-f", jsonFile}),
		methods: []string{http.MethodPost, http.MethodPatch, http.MethodDelete},
		output:  "R-Content-Type: application/octet-stream\nR-Method: %s\n" + jsonContent,
	},
	{ // case 10
		args:    convertToFunctionArgs([]string{"-h", "Content-Type: application/json; charset=utf-8", "-f", jsonFile}),
		methods: []string{http.MethodPost, http.MethodPatch, http.MethodDelete},
		output:  "R-Content-Type: application/json; charset=utf-8\nR-Method: %s\n" + jsonContent,
	},
	{ // case 11
		args:    convertToFunctionArgs([]string{"-h", "Content-Type: application/json; charset=utf-8", "-f", jsonFile}),
		methods: []string{http.MethodPut},
		output:  "R-Body: " + jsonContent + "\nR-Content-Type: application/json; charset=utf-8\nR-Method: %s\n",
	},
}

func TestHttpFunction(t *testing.T) {
	server := getTestServer()
	for i, tc := range getTestCase {
		tc.args = append(tc.args, &args.FunctionArg{
			Val:  server.URL,
			Kind: reflect.String,
		})
		for _, method := range tc.methods {
			fn := GetFunction(strings.ToLower(method))
			t.Logf("TestGet case #%d (%s)", i+1, method)
			result, err := fn.Apply(tc.args)
			if tc.err {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				data, err := responseResponse(result, method)
				require.NoError(t, err)
				assert.Equal(t, fmt.Sprintf(tc.output.(string), method), string(data))
			}
		}
	}
}
