package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

/* --------------------------------------------------------------------
   Configuration (env vars → sane defaults)
-------------------------------------------------------------------- */

var (
	listenAddr              = env("LISTEN_ADDR", ":8082")
	upstreamAddr            = env("UPSTREAM_ADDR", "https://127.0.0.1:8080")
	authEndpoint            = env("AUTH_ENDPOINT", "http://127.0.0.1:8181/v1/signature")
	authMethod              = env("AUTH_METHOD", http.MethodPost)
	maxBodySize             = envInt("MAX_BODY_SIZE_MB", 30)               // MB forwarded to auth
	debug                   = envBool("DEBUG", false)                      // DEBUG=1 turns on verbose logs
	authIncludeRegex        = env("AUTH_INCLUDE_REGEX", "^/public(?:/|$)") // only URIs matching this require auth
	forwardAuthHeadersRaw   = env("AUTH_FORWARD_AUTH_HEADERS", "")         // comma header list; empty → no Auth headers forwarding
	upstreamURL             *url.URL                                       // parsed once in init()
	authIncludeRE           *regexp.Regexp                                 // compiled once in init()
	forwardAuthHeadersCanon []string                                       // canonicalized header names
)

func init() {
	u, err := url.Parse(upstreamAddr)
	if err != nil {
		log.Fatalf("invalid UPSTREAM_ADDR %q: %v", upstreamAddr, err)
	}
	upstreamURL = u

	re, err := regexp.Compile(authIncludeRegex)
	if err != nil {
		log.Fatalf("invalid AUTH_INCLUDE_REGEX %q: %v", authIncludeRegex, err)
	}
	authIncludeRE = re

	if strings.TrimSpace(forwardAuthHeadersRaw) != "" {
		forwardAuthHeadersCanon = parseHeaderList(forwardAuthHeadersRaw)
	}
}

/* --------------------------------------------------------------------
   Main – tiny reverse proxy with auth pre-check
-------------------------------------------------------------------- */

func main() {
	proxy := httputil.ReverseProxy{
		Director:  func(*http.Request) {}, // keep original URL unchanged
		Transport: &authTransport{http.DefaultTransport},
		ErrorLog:  log.New(log.Writer(), "[proxy] ", log.LstdFlags),
	}

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           &proxy,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("auth-proxy listening on %s  (→  %s)", listenAddr, upstreamAddr)
	log.Fatal(srv.ListenAndServe())
}

/* --------------------------------------------------------------------
   The smart RoundTripper that calls AUTH before UPSTREAM
-------------------------------------------------------------------- */

type authTransport struct{ upstream http.RoundTripper }

func (a *authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	/* ---------- BYPASS CHECK ---------- */
	if !authIncludeRE.MatchString(r.URL.Path) || r.Method == http.MethodOptions {
		// forward directly to upstream without touching auth
		out := r.Clone(r.Context())
		out.URL.Scheme = upstreamURL.Scheme
		out.URL.Host = upstreamURL.Host
		out.Host = upstreamURL.Host
		if debug {
			dump("BYPASS → UPSTREAM", out, nil, "")
		}
		return a.upstream.RoundTrip(out)
	}

	/* ---------- CLIENT → PROXY ---------- */
	if debug {
		dump("CLIENT → PROXY", r, nil, "")
	}

	// 1) Copy/peek body so we can use it twice
	var bodyCopy []byte
	if r.Body != nil {
		peek, _ := io.ReadAll(io.LimitReader(r.Body, int64(maxBodySize<<20)))
		bodyCopy = peek
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), r.Body))
	}

	// 2) Call auth service
	authReq, _ := http.NewRequest(authMethod, authEndpoint, bytes.NewReader(bodyCopy))
	authReq.Header = cloneSubset(r.Header)
	authReq.Header.Set("X-Orig-Uri", r.URL.RequestURI())
	authReq.Header.Set("X-Orig-Method", r.Method)

	if debug {
		dump("PROXY → AUTH", authReq, nil, string(bodyCopy))
	}

	authResp, err := a.upstream.RoundTrip(authReq)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(authResp.Body)

	if debug {
		dump("AUTH → PROXY", authReq, authResp, "")
	}

	// 3) Allow / deny
	if authResp.StatusCode != http.StatusOK {
		return &http.Response{
			StatusCode: authResp.StatusCode,
			Status:     authResp.Status,
			Proto:      "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Request: r,
			Header:  authResp.Header.Clone(),
			Body:    io.NopCloser(bytes.NewReader(nil)),
		}, nil
	}

	// 4) Forward original request to upstream
	out := r.Clone(r.Context())
	out.URL.Scheme = upstreamURL.Scheme
	out.URL.Host = upstreamURL.Host
	out.Host = upstreamURL.Host

	// 4a) Copy configured headers from AUTH response → upstream request (optional)
	if len(forwardAuthHeadersCanon) > 0 {
		for _, h := range forwardAuthHeadersCanon {
			values := authResp.Header.Values(h)
			if len(values) == 0 {
				continue
			}
			out.Header.Del(h) // replace any existing values
			for _, v := range values {
				if v != "" {
					out.Header.Add(h, v)
				}
			}
		}
		if debug {
			log.Printf("[FORWARD] copied headers from AUTH → UPSTREAM: %s", strings.Join(forwardAuthHeadersCanon, ", "))
		}
	}

	if debug {
		dump("PROXY → UPSTREAM", out, nil, "")
	}

	return a.upstream.RoundTrip(out)
}

/* --------------------------------------------------------------------
   Pretty text-logging helpers
-------------------------------------------------------------------- */

func dump(tag string, req *http.Request, resp *http.Response, bodyPreview string) {
	log.Println("------------------------------------------------------------")
	log.Printf("[%s] %s %s", tag, req.Method, req.URL.RequestURI())

	// request headers
	for k, v := range req.Header {
		log.Printf("  > %s: %s", k, strings.Join(v, ", "))
	}
	if bodyPreview != "" {
		log.Printf("  > body (%d bytes): %q", len(bodyPreview), trimNL(bodyPreview))
	}

	if resp != nil {
		log.Printf("  < %s", resp.Status)
		for k, v := range resp.Header {
			log.Printf("  < %s: %s", k, strings.Join(v, ", "))
		}
	}
}

func trimNL(s string) string { return strings.ReplaceAll(s, "\n", "\\n") }

/* --------------------------------------------------------------------
   Utility helpers
-------------------------------------------------------------------- */

func cloneSubset(src http.Header) http.Header {
	dst := http.Header{}
	for k, v := range src {
		switch http.CanonicalHeaderKey(k) {
		case "Signature", "Signature-Date":
			dst[k] = v
		}
	}
	return dst
}

func parseHeaderList(raw string) []string {
	items := strings.Split(raw, ",")
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		h := http.CanonicalHeaderKey(strings.TrimSpace(it))
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

func drainAndClose(c io.ReadCloser) { io.Copy(io.Discard, io.LimitReader(c, 4<<10)); _ = c.Close() }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes":
			return true
		case "0", "false", "no":
			return false
		}
	}
	return def
}
