package common

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/api/common/test"
	rpctest "github.com/ipfs-cluster/ipfs-cluster/test"

	libp2p "github.com/libp2p/go-libp2p"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	SSLCertFile         = "test/server.crt"
	SSLKeyFile          = "test/server.key"
	validUserName       = "validUserName"
	validUserPassword   = "validUserPassword"
	adminUserName       = "adminUserName"
	adminUserPassword   = "adminUserPassword"
	invalidUserName     = "invalidUserName"
	invalidUserPassword = "invalidUserPassword"
)

var (
	validToken, _   = generateSignedTokenString(validUserName, validUserPassword)
	invalidToken, _ = generateSignedTokenString(invalidUserName, invalidUserPassword)
)

func routes(c *rpc.Client) []Route {
	return []Route{
		{
			"Test",
			"GET",
			"/test",
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add("Content-Type", "application/json")
				w.Write([]byte(`{ "thisis": "atest" }`))
			},
		},
	}

}

func testAPIwithConfig(t *testing.T, cfg *Config, name string) *API {
	ctx := context.Background()
	apiMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	h, err := libp2p.New(libp2p.ListenAddrs(apiMAddr))
	if err != nil {
		t.Fatal(err)
	}

	cfg.HTTPListenAddr = []ma.Multiaddr{apiMAddr}

	rest, err := NewAPIWithHost(ctx, cfg, h, routes)
	if err != nil {
		t.Fatalf("should be able to create a new %s API: %s", name, err)
	}

	// No keep alive for tests
	rest.server.SetKeepAlivesEnabled(false)
	rest.SetClient(rpctest.NewMockRPCClient(t))

	return rest
}

func testAPI(t *testing.T) *API {
	cfg := newDefaultTestConfig(t)
	cfg.CORSAllowedOrigins = []string{test.ClientOrigin}
	cfg.CORSAllowedMethods = []string{"GET", "POST", "DELETE"}
	//cfg.CORSAllowedHeaders = []string{"Content-Type"}
	cfg.CORSMaxAge = 10 * time.Minute

	return testAPIwithConfig(t, cfg, "basic")
}

func testHTTPSAPI(t *testing.T) *API {
	cfg := newDefaultTestConfig(t)
	cfg.PathSSLCertFile = SSLCertFile
	cfg.PathSSLKeyFile = SSLKeyFile
	var err error
	cfg.TLS, err = newTLSConfig(cfg.PathSSLCertFile, cfg.PathSSLKeyFile)
	if err != nil {
		t.Fatal(err)
	}

	return testAPIwithConfig(t, cfg, "https")
}

func testAPIwithBasicAuth(t *testing.T) *API {
	cfg := newDefaultTestConfig(t)
	cfg.BasicAuthCredentials = map[string]string{
		validUserName: validUserPassword,
		adminUserName: adminUserPassword,
	}

	return testAPIwithConfig(t, cfg, "Basic Authentication")
}

func TestAPIShutdown(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	err := rest.Shutdown(ctx)
	if err != nil {
		t.Error("should shutdown cleanly: ", err)
	}
	// test shutting down twice
	rest.Shutdown(ctx)

}

func TestHTTPSTestEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	httpsrest := testHTTPSAPI(t)
	defer rest.Shutdown(ctx)
	defer httpsrest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		r := make(map[string]string)
		test.MakeGet(t, rest, url(rest)+"/test", &r)
		if r["thisis"] != "atest" {
			t.Error("expected correct body")
		}
	}

	httpstf := func(t *testing.T, url test.URLFunc) {
		r := make(map[string]string)
		test.MakeGet(t, httpsrest, url(httpsrest)+"/test", &r)
		if r["thisis"] != "atest" {
			t.Error("expected correct body")
		}
	}

	test.BothEndpoints(t, tf)
	test.HTTPSEndPoint(t, httpstf)
}

func TestAPILogging(t *testing.T) {
	ctx := context.Background()
	cfg := newDefaultTestConfig(t)

	logFile, err := filepath.Abs("http.log")
	if err != nil {
		t.Fatal(err)
	}
	cfg.HTTPLogFile = logFile

	rest := testAPIwithConfig(t, cfg, "log_enabled")
	defer os.Remove(cfg.HTTPLogFile)

	info, err := os.Stat(cfg.HTTPLogFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 0 {
		t.Errorf("expected empty log file")
	}

	id := api.ID{}
	test.MakeGet(t, rest, test.HTTPURL(rest)+"/test", &id)

	info, err = os.Stat(cfg.HTTPLogFile)
	if err != nil {
		t.Fatal(err)
	}
	size1 := info.Size()
	if size1 == 0 {
		t.Error("did not expect an empty log file")
	}

	// Restart API and make sure that logs are being appended
	rest.Shutdown(ctx)

	rest = testAPIwithConfig(t, cfg, "log_enabled")
	defer rest.Shutdown(ctx)

	test.MakeGet(t, rest, test.HTTPURL(rest)+"/id", &id)

	info, err = os.Stat(cfg.HTTPLogFile)
	if err != nil {
		t.Fatal(err)
	}
	size2 := info.Size()
	if size2 == 0 {
		t.Error("did not expect an empty log file")
	}

	if !(size2 > size1) {
		t.Error("logs were not appended")
	}

}

func TestNotFoundHandler(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		bytes := make([]byte, 10)
		for i := 0; i < 10; i++ {
			bytes[i] = byte(65 + rand.Intn(25)) //A=65 and Z = 65+25
		}

		var errResp api.Error
		test.MakePost(t, rest, url(rest)+"/"+string(bytes), []byte{}, &errResp)
		if errResp.Code != 404 {
			t.Errorf("expected error not found: %+v", errResp)
		}

		var errResp1 api.Error
		test.MakeGet(t, rest, url(rest)+"/"+string(bytes), &errResp1)
		if errResp1.Code != 404 {
			t.Errorf("expected error not found: %+v", errResp)
		}
	}

	test.BothEndpoints(t, tf)
}

func TestCORS(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	type testcase struct {
		method string
		path   string
	}

	tf := func(t *testing.T, url test.URLFunc) {
		reqHeaders := make(http.Header)
		reqHeaders.Set("Origin", "myorigin")
		reqHeaders.Set("Access-Control-Request-Headers", "content-type")

		for _, tc := range []testcase{
			{"GET", "/test"},
			//			testcase{},
		} {
			reqHeaders.Set("Access-Control-Request-Method", tc.method)
			headers := test.MakeOptions(t, rest, url(rest)+tc.path, reqHeaders)
			aorigin := headers.Get("Access-Control-Allow-Origin")
			amethods := headers.Get("Access-Control-Allow-Methods")
			aheaders := headers.Get("Access-Control-Allow-Headers")
			acreds := headers.Get("Access-Control-Allow-Credentials")
			maxage := headers.Get("Access-Control-Max-Age")

			if aorigin != "myorigin" {
				t.Error("Bad ACA-Origin:", aorigin)
			}

			if amethods != tc.method {
				t.Error("Bad ACA-Methods:", amethods)
			}

			if aheaders != "content-type" {
				t.Error("Bad ACA-Headers:", aheaders)
			}

			if acreds != "true" {
				t.Error("Bad ACA-Credentials:", acreds)
			}

			if maxage != "600" {
				t.Error("Bad AC-Max-Age:", maxage)
			}
		}

	}

	test.BothEndpoints(t, tf)
}

type responseChecker func(*http.Response) error
type requestShaper func(*http.Request) error

type httpTestcase struct {
	method  string
	path    string
	header  http.Header
	body    io.ReadCloser
	shaper  requestShaper
	checker responseChecker
}

func httpStatusCodeChecker(resp *http.Response, expectedStatus int) error {
	if resp.StatusCode == expectedStatus {
		return nil
	}
	return fmt.Errorf("unexpected HTTP status code: %d", resp.StatusCode)
}

func assertHTTPStatusIsUnauthoriazed(resp *http.Response) error {
	return httpStatusCodeChecker(resp, http.StatusUnauthorized)
}

func assertHTTPStatusIsTooLarge(resp *http.Response) error {
	return httpStatusCodeChecker(resp, http.StatusRequestHeaderFieldsTooLarge)
}

func makeHTTPStatusNegatedAssert(checker responseChecker) responseChecker {
	return func(resp *http.Response) error {
		if checker(resp) == nil {
			return fmt.Errorf("unexpected HTTP status code: %d", resp.StatusCode)
		}
		return nil
	}
}

func (tc *httpTestcase) getTestFunction(api *API) test.Func {
	return func(t *testing.T, prefixMaker test.URLFunc) {
		h := test.MakeHost(t, api)
		defer h.Close()
		url := prefixMaker(api) + tc.path
		c := test.HTTPClient(t, h, test.IsHTTPS(url))
		req, err := http.NewRequest(tc.method, url, tc.body)
		if err != nil {
			t.Fatal("Failed to assemble a HTTP request: ", err)
		}
		if tc.header != nil {
			req.Header = tc.header
		}
		if tc.shaper != nil {
			err := tc.shaper(req)
			if err != nil {
				t.Fatal("Failed to shape a HTTP request: ", err)
			}
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal("Failed to make a HTTP request: ", err)
		}
		if tc.checker != nil {
			if err := tc.checker(resp); err != nil {
				r, e := httputil.DumpRequest(req, true)
				if e != nil {
					t.Errorf("Assertion failed with: %q", err)
				} else {
					t.Errorf("Assertion failed with: %q on request: \n%.100s", err, r)
				}
			}
		}
	}
}

func makeBasicAuthRequestShaper(username, password string) requestShaper {
	return func(req *http.Request) error {
		req.SetBasicAuth(username, password)
		return nil
	}
}

func makeTokenAuthRequestShaper(token string) requestShaper {
	return func(req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

func makeLongHeaderShaper(size int) requestShaper {
	return func(req *http.Request) error {
		for sz := size; sz > 0; sz -= 8 {
			req.Header.Add("Foo", "bar")
		}
		return nil
	}
}

func TestBasicAuth(t *testing.T) {
	ctx := context.Background()
	rest := testAPIwithBasicAuth(t)
	defer rest.Shutdown(ctx)

	for _, tc := range []httpTestcase{
		{},
		{
			method:  "",
			path:    "",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "POST",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "DELETE",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "HEAD",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "OPTIONS", // Always allowed for CORS
			path:    "/foo",
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "PUT",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "TRACE",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "CONNECT",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "BAR",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeBasicAuthRequestShaper(invalidUserName, invalidUserPassword),
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeBasicAuthRequestShaper(validUserName, invalidUserPassword),
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeBasicAuthRequestShaper(invalidUserName, validUserPassword),
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeBasicAuthRequestShaper(adminUserName, validUserPassword),
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeBasicAuthRequestShaper(validUserName, validUserPassword),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "POST",
			path:    "/foo",
			shaper:  makeBasicAuthRequestShaper(validUserName, validUserPassword),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "DELETE",
			path:    "/foo",
			shaper:  makeBasicAuthRequestShaper(validUserName, validUserPassword),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "BAR",
			path:    "/foo",
			shaper:  makeBasicAuthRequestShaper(validUserName, validUserPassword),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "GET",
			path:    "/test",
			shaper:  makeBasicAuthRequestShaper(validUserName, validUserPassword),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
	} {
		test.BothEndpoints(t, tc.getTestFunction(rest))
	}
}

func TestTokenAuth(t *testing.T) {
	ctx := context.Background()
	rest := testAPIwithBasicAuth(t)
	defer rest.Shutdown(ctx)

	for _, tc := range []httpTestcase{
		{},
		{
			method:  "",
			path:    "",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "POST",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "DELETE",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "HEAD",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "OPTIONS", // Always allowed for CORS
			path:    "/foo",
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "PUT",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "TRACE",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "CONNECT",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "BAR",
			path:    "/foo",
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeTokenAuthRequestShaper(invalidToken),
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeTokenAuthRequestShaper(invalidToken),
			checker: assertHTTPStatusIsUnauthoriazed,
		},
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeTokenAuthRequestShaper(validToken),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "POST",
			path:    "/foo",
			shaper:  makeTokenAuthRequestShaper(validToken),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "DELETE",
			path:    "/foo",
			shaper:  makeTokenAuthRequestShaper(validToken),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "BAR",
			path:    "/foo",
			shaper:  makeTokenAuthRequestShaper(validToken),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
		{
			method:  "GET",
			path:    "/test",
			shaper:  makeTokenAuthRequestShaper(validToken),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsUnauthoriazed),
		},
	} {
		test.BothEndpoints(t, tc.getTestFunction(rest))
	}
}

func TestLimitMaxHeaderSize(t *testing.T) {
	maxHeaderBytes := 4 * DefaultMaxHeaderBytes
	cfg := newTestConfig()
	cfg.MaxHeaderBytes = maxHeaderBytes
	ctx := context.Background()
	rest := testAPIwithConfig(t, cfg, "http with maxHeaderBytes")
	defer rest.Shutdown(ctx)

	for _, tc := range []httpTestcase{
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeLongHeaderShaper(maxHeaderBytes * 2),
			checker: assertHTTPStatusIsTooLarge,
		},
		{
			method:  "GET",
			path:    "/foo",
			shaper:  makeLongHeaderShaper(maxHeaderBytes / 2),
			checker: makeHTTPStatusNegatedAssert(assertHTTPStatusIsTooLarge),
		},
	} {
		test.BothEndpoints(t, tc.getTestFunction(rest))
	}
}
