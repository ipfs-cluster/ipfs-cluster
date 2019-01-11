package ipfsproxy

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ipfs/ipfs-cluster/version"
)

// This file has the collection of header-related functions

// We will extract all these from a pre-flight OPTIONs request to IPFS to
// use in the respose of a hijacked request (usually POST).
var corsHeaders = []string{
	// These two must be returned as IPFS would return them
	// for a request with the same origin.
	"Access-Control-Allow-Origin",
	"Vary", // seems more correctly set in OPTIONS than other requests.

	// This is returned by OPTIONS so we can take it, even if ipfs sets
	// it for nothing by default.
	"Access-Control-Allow-Credentials",

	// Unfortunately this one should not come with OPTIONS by default,
	// but only with the real request itself.
	// We use extractHeadersDefault for it, even though I think
	// IPFS puts it in OPTIONS responses too. In any case, ipfs
	// puts it on all requests as of 0.4.18, so it should be OK.
	// "Access-Control-Expose-Headers",

	// Only for preflight responses, we do not need
	// these since we will simply proxy OPTIONS requests and not
	// handle them.
	//
	// They are here for reference about other CORS related headers.
	// "Access-Control-Max-Age",
	// "Access-Control-Allow-Methods",
	// "Access-Control-Allow-Headers",
}

// This can be used to hardcode header extraction from the proxy if we ever
// need to. It is appended to config.ExtractHeaderExtra.
// Maybe "X-Ipfs-Gateway" is a good candidate.
var extractHeadersDefault = []string{
	"Access-Control-Expose-Headers",
}

const ipfsHeadersTimestampKey = "proxyHeadersTS"

// ipfsHeaders returns all the headers we want to extract-once from IPFS: a
// concatenation of extractHeadersDefault and config.ExtractHeadersExtra.
func (proxy *Server) ipfsHeaders() []string {
	return append(extractHeadersDefault, proxy.config.ExtractHeadersExtra...)
}

// rememberIPFSHeaders extracts headers and stores them for re-use with
// setIPFSHeaders.
func (proxy *Server) rememberIPFSHeaders(hdrs http.Header) {
	for _, h := range proxy.ipfsHeaders() {
		proxy.ipfsHeadersStore.Store(h, hdrs[h])
	}
	// use the sync map to store the ts
	proxy.ipfsHeadersStore.Store(ipfsHeadersTimestampKey, time.Now())
}

// returns whether we can consider that whatever headers we are
// storing have a valid TTL still.
func (proxy *Server) headersWithinTTL() bool {
	ttl := proxy.config.ExtractHeadersTTL
	if ttl == 0 {
		return true
	}

	tsRaw, ok := proxy.ipfsHeadersStore.Load(ipfsHeadersTimestampKey)
	if !ok {
		return false
	}

	ts, ok := tsRaw.(time.Time)
	if !ok {
		return false
	}

	lifespan := time.Since(ts)
	return lifespan < ttl
}

// rememberIPFSHeaders adds the known IPFS Headers to the destination
// and returns true if we could set all the headers in the list and
// the TTL has not expired.
// False is used to determine if we need to make a request to try
// to extract these headers.
func (proxy *Server) setIPFSHeaders(dest http.Header) bool {
	r := true

	if !proxy.headersWithinTTL() {
		r = false
		// still set those headers we can set in the destination.
		// We do our best there, since maybe the ipfs daemon
		// is down and what we have now is all we can use.
	}

	for _, h := range proxy.ipfsHeaders() {
		v, ok := proxy.ipfsHeadersStore.Load(h)
		if !ok {
			r = false
			continue
		}
		dest[h] = v.([]string)
	}
	return r
}

// copyHeadersFromIPFSWithRequest makes a request to IPFS as used by the proxy
// and copies the given list of hdrs from the response to the dest http.Header
// object.
func (proxy *Server) copyHeadersFromIPFSWithRequest(
	hdrs []string,
	dest http.Header, req *http.Request,
) error {
	res, err := proxy.ipfsRoundTripper.RoundTrip(req)
	if err != nil {
		logger.Error("error making request for header extraction to ipfs: ", err)
		return err
	}

	for _, h := range hdrs {
		dest[h] = res.Header[h]
	}
	return nil
}

// setHeaders sets some headers for all hijacked endpoints:
// - First we fix CORs headers by making an OPTIONS request to IPFS with the
//   same Origin. Our objective is to get headers for non-preflight requests
//   only (the ones we hijack).
// - Second we add any of the one-time-extracted headers that we deem necessary
//   or the user needs from IPFS (in case of custom headers), or i by ours.
//   This may trigger a single POST request to ExtractHeaderPath if they
//   were not extracted before.
// - Third we set our own headers.
func (proxy *Server) setHeaders(dest http.Header, srcRequest *http.Request) {
	proxy.setCORSHeaders(dest, srcRequest)
	proxy.setAdditionalIpfsHeaders(dest, srcRequest)
	proxy.setClusterProxyHeaders(dest, srcRequest)
}

// see setHeaders
func (proxy *Server) setCORSHeaders(dest http.Header, srcRequest *http.Request) {
	// Fix CORS headers by making an OPTIONS request

	// The request URL only has a valid Path(). See http.Request docs.
	srcURL := fmt.Sprintf("%s%s", proxy.nodeAddr, srcRequest.URL.Path)
	req, err := http.NewRequest(http.MethodOptions, srcURL, nil)
	if err != nil { // this should really not happen.
		logger.Error(err)
		return
	}

	req.Header["Origin"] = srcRequest.Header["Origin"]
	req.Header.Set("Access-Control-Request-Method", srcRequest.Method)
	// error is logged. We proceed if request failed.
	proxy.copyHeadersFromIPFSWithRequest(corsHeaders, dest, req)
}

// see setHeaders
func (proxy *Server) setAdditionalIpfsHeaders(dest http.Header, srcRequest *http.Request) {
	// Avoid re-requesting these if we have them
	if ok := proxy.setIPFSHeaders(dest); ok {
		return
	}

	srcURL := fmt.Sprintf("%s%s", proxy.nodeAddr, proxy.config.ExtractHeadersPath)
	req, err := http.NewRequest(http.MethodPost, srcURL, nil)
	if err != nil {
		logger.Error("error extracting additional headers from ipfs", err)
		return
	}
	// error is logged. We proceed if request failed.
	proxy.copyHeadersFromIPFSWithRequest(
		proxy.ipfsHeaders(),
		dest,
		req,
	)
	proxy.rememberIPFSHeaders(dest)
}

// see setHeaders
func (proxy *Server) setClusterProxyHeaders(dest http.Header, srcRequest *http.Request) {
	dest.Set("Content-Type", "application/json")
	dest.Set("Server", fmt.Sprintf("ipfs-cluster/ipfsproxy/%s", version.Version))
}
