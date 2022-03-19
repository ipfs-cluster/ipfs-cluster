// Package test provides utility methods to test APIs based on the common
// API.
package test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	peerstore "github.com/libp2p/go-libp2p-core/peerstore"
	p2phttp "github.com/libp2p/go-libp2p-http"
)

var (
	// SSLCertFile is the location of the certificate file.
	// Used in HTTPClient to set the right certificate when
	// creating an HTTPs client. Might need adjusting depending
	// on where the tests are running.
	SSLCertFile = "test/server.crt"

	// ClientOrigin sets the Origin header for requests to this.
	ClientOrigin = "myorigin"
)

// ProcessResp puts a response into a given type or fails the test.
func ProcessResp(t *testing.T, httpResp *http.Response, err error, resp interface{}) {
	if err != nil {
		t.Fatal("error making request: ", err)
	}
	body, err := ioutil.ReadAll(httpResp.Body)
	defer httpResp.Body.Close()
	if err != nil {
		t.Fatal("error reading body: ", err)
	}
	if len(body) != 0 {
		err = json.Unmarshal(body, resp)
		if err != nil {
			t.Error(string(body))
			t.Fatal("error parsing json: ", err)
		}
	}
}

// ProcessStreamingResp decodes a streaming response into the given type
// and fails the test on error.
func ProcessStreamingResp(t *testing.T, httpResp *http.Response, err error, resp interface{}) {
	if err != nil {
		t.Fatal("error making streaming request: ", err)
	}

	if httpResp.StatusCode > 399 {
		// normal response with error
		ProcessResp(t, httpResp, err, resp)
		return
	}

	defer httpResp.Body.Close()
	dec := json.NewDecoder(httpResp.Body)

	// If we passed a slice we fill it in, otherwise we just decode
	// on top of the passed value.
	tResp := reflect.TypeOf(resp)
	if tResp.Elem().Kind() == reflect.Slice {
		vSlice := reflect.MakeSlice(reflect.TypeOf(resp).Elem(), 0, 1000)
		vType := tResp.Elem().Elem()
		for {
			v := reflect.New(vType)
			err := dec.Decode(v.Interface())
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			vSlice = reflect.Append(vSlice, v.Elem())
		}
		reflect.ValueOf(resp).Elem().Set(vSlice)
	} else {
		for {
			err := dec.Decode(resp)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

// CheckHeaders checks that all the headers are set to what is expected.
func CheckHeaders(t *testing.T, expected map[string][]string, url string, headers http.Header) {
	for k, v := range expected {
		if strings.Join(v, ",") != strings.Join(headers[k], ",") {
			t.Errorf("%s does not show configured headers: %s", url, k)
		}
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Errorf("%s is not application/json", url)
	}

	if eh := headers.Get("Access-Control-Expose-Headers"); eh == "" {
		t.Error("AC-Expose-Headers not set")
	}
}

// This represents what an API is to us.
type API interface {
	HTTPAddresses() ([]string, error)
	Host() host.Host
	Headers() map[string][]string
}

// URLFunc is a function that given an API returns a url string.
type URLFunc func(a API) string

// HTTPURL returns the http endpoint of the API.
func HTTPURL(a API) string {
	u, _ := a.HTTPAddresses()
	return fmt.Sprintf("http://%s", u[0])
}

// P2pURL returns the libp2p endpoint of the API.
func P2pURL(a API) string {
	return fmt.Sprintf("libp2p://%s", peer.Encode(a.Host().ID()))
}

// HttpsURL returns the HTTPS endpoint of the API
func httpsURL(a API) string {
	u, _ := a.HTTPAddresses()
	return fmt.Sprintf("https://%s", u[0])
}

// IsHTTPS returns true if a url string uses HTTPS.
func IsHTTPS(url string) bool {
	return strings.HasPrefix(url, "https")
}

// HTTPClient returns a client that supporst both http/https and
// libp2p-tunneled-http.
func HTTPClient(t *testing.T, h host.Host, isHTTPS bool) *http.Client {
	tr := &http.Transport{}
	if isHTTPS {
		certpool := x509.NewCertPool()
		cert, err := ioutil.ReadFile(SSLCertFile)
		if err != nil {
			t.Fatal("error reading cert for https client: ", err)
		}
		certpool.AppendCertsFromPEM(cert)
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certpool,
			}}
	}
	if h != nil {
		tr.RegisterProtocol("libp2p", p2phttp.NewTransport(h))
	}
	return &http.Client{Transport: tr}
}

// MakeHost makes a libp2p host that knows how to talk to the given API.
func MakeHost(t *testing.T, api API) host.Host {
	h, err := libp2p.New()
	if err != nil {
		t.Fatal(err)
	}
	h.Peerstore().AddAddrs(
		api.Host().ID(),
		api.Host().Addrs(),
		peerstore.PermanentAddrTTL,
	)
	return h
}

// MakeGet performs a GET request against the API.
func MakeGet(t *testing.T, api API, url string, resp interface{}) {
	h := MakeHost(t, api)
	defer h.Close()
	c := HTTPClient(t, h, IsHTTPS(url))
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Origin", ClientOrigin)
	httpResp, err := c.Do(req)
	ProcessResp(t, httpResp, err, resp)
	CheckHeaders(t, api.Headers(), url, httpResp.Header)
}

// MakePost performs a POST request agains the API with the given body.
func MakePost(t *testing.T, api API, url string, body []byte, resp interface{}) {
	MakePostWithContentType(t, api, url, body, "application/json", resp)
}

// MakePostWithContentType performs a POST with the given body and content-type.
func MakePostWithContentType(t *testing.T, api API, url string, body []byte, contentType string, resp interface{}) {
	h := MakeHost(t, api)
	defer h.Close()
	c := HTTPClient(t, h, IsHTTPS(url))
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Origin", ClientOrigin)
	httpResp, err := c.Do(req)
	ProcessResp(t, httpResp, err, resp)
	CheckHeaders(t, api.Headers(), url, httpResp.Header)
}

// MakeDelete performs a DELETE request against the given API.
func MakeDelete(t *testing.T, api API, url string, resp interface{}) {
	h := MakeHost(t, api)
	defer h.Close()
	c := HTTPClient(t, h, IsHTTPS(url))
	req, _ := http.NewRequest(http.MethodDelete, url, bytes.NewReader([]byte{}))
	req.Header.Set("Origin", ClientOrigin)
	httpResp, err := c.Do(req)
	ProcessResp(t, httpResp, err, resp)
	CheckHeaders(t, api.Headers(), url, httpResp.Header)
}

// MakeOptions performs an OPTIONS request against the given api.
func MakeOptions(t *testing.T, api API, url string, reqHeaders http.Header) http.Header {
	h := MakeHost(t, api)
	defer h.Close()
	c := HTTPClient(t, h, IsHTTPS(url))
	req, _ := http.NewRequest(http.MethodOptions, url, nil)
	req.Header = reqHeaders
	httpResp, err := c.Do(req)
	ProcessResp(t, httpResp, err, nil)
	return httpResp.Header
}

// MakeStreamingPost performs a POST request and uses ProcessStreamingResp
func MakeStreamingPost(t *testing.T, api API, url string, body io.Reader, contentType string, resp interface{}) {
	h := MakeHost(t, api)
	defer h.Close()
	c := HTTPClient(t, h, IsHTTPS(url))
	req, _ := http.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Origin", ClientOrigin)
	httpResp, err := c.Do(req)
	ProcessStreamingResp(t, httpResp, err, resp)
	CheckHeaders(t, api.Headers(), url, httpResp.Header)
}

// MakeStreamingGet performs a GET request and uses ProcessStreamingResp
func MakeStreamingGet(t *testing.T, api API, url string, resp interface{}) {
	h := MakeHost(t, api)
	defer h.Close()
	c := HTTPClient(t, h, IsHTTPS(url))
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Origin", ClientOrigin)
	httpResp, err := c.Do(req)
	ProcessStreamingResp(t, httpResp, err, resp)
	CheckHeaders(t, api.Headers(), url, httpResp.Header)
}

// Func is a function that runs a test with a given URL.
type Func func(t *testing.T, url URLFunc)

// BothEndpoints runs a test.Func against the http and p2p endpoints.
func BothEndpoints(t *testing.T, test Func) {
	t.Run("in-parallel", func(t *testing.T) {
		t.Run("http", func(t *testing.T) {
			t.Parallel()
			test(t, HTTPURL)
		})
		t.Run("libp2p", func(t *testing.T) {
			t.Parallel()
			test(t, P2pURL)
		})
	})
}

// HTTPSEndpoint runs the given test.Func against an HTTPs endpoint.
func HTTPSEndPoint(t *testing.T, test Func) {
	t.Run("in-parallel", func(t *testing.T) {
		t.Run("https", func(t *testing.T) {
			t.Parallel()
			test(t, httpsURL)
		})
	})
}
