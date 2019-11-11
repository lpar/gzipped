package gzipped

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"testing"
)

// foundEncoding checks if a given encoding was seen in the acceptEncodings list
func foundEncoding(acceptEncodings []encoding, target string) bool {
	for _, seen := range acceptEncodings {
		if seen.name == target {
			return true
		}
	}
	return false
}

// Test various cases where gzip is or is not acceptable
func TestGzipAcceptable(t *testing.T) {
	req := http.Request{Header: http.Header{}}
	for _, info := range []struct {
		hdr    string // the Accept-Encoding header
		expect bool   // whether we expect gzip to be acceptable
	}{
		{"gzip", true},
		{"*", true},
		{"compress,gzip;q=0.1", true},
		{"compress, * ;q=1", true},
		{"deflate ;q=0.3, gzip ;q=0.7, x-foo", true},
		{"gzip;q=0,*", false},
		{"deflate; gzip ;q=0 , x-foo,*", false},
	} {
		req.Header.Set("Accept-Encoding", info.hdr)
		accepted := foundEncoding(acceptable(&req), "gzip")
		if accepted != info.expect {
			t.Errorf("expected gzip accept to be %t, instead got %t, for header %s", info.expect, accepted, info.hdr)
		}
	}
}

// Test that the server respects client preferences
func TestPreference(t *testing.T) {
	req := http.Request{Header: http.Header{}}

	// the client doesn't set any preferences, so we should pick br
	for _, info := range []struct {
		hdr    string // the Accept-Encoding string
		expect string // the expected encoding chosen by the server
	}{
		{"*", "br"},
		{"gzip, deflate, br", "br"},
		{"gzip, deflate, br;q=0.5", "gzip"},
	} {
		req.Header.Set("Accept-Encoding", info.hdr)
		acceptEncodings := acceptable(&req)
		if len(acceptEncodings) == 0 {
			t.Errorf("server failed to find an accept encoding")
			continue
		}
		best := acceptEncodings[0].name
		if best != info.expect {
			t.Errorf("server chose %s but we expected %s for header %s", best, info.expect, info.hdr)
		}
	}
}

func testGet(t *testing.T, withGzip bool, expectedBody string) {
	fs := FileServer(http.Dir("./testdata/"))
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/file.txt", nil)
	var encoding string
	if withGzip {
		encoding = " with gzip"
		req.Header.Set("Accept-Encoding", "gzip,*")
	} else {
		encoding = ""
	}
	fs.ServeHTTP(rr, req)
	h := rr.Header()
	if h["Content-Type"][0] != "text/plain; charset=utf-8" {
		t.Errorf("GET returned wrong content type %s", h["Content-Type"])
	}
	clh := h["Content-Length"]
	// Check the content-length is correct.
	if len(clh) > 0 {
		bytes, err := strconv.Atoi(clh[0])
		if err != nil {
			t.Errorf("Invalid Content-Length on response: '%s'", clh[0])
		}
		n := rr.Body.Len()
		if n != bytes {
			t.Errorf("GET expected %d bytes, got %d", bytes, n)
		}
	}
	var body string
	if withGzip {
		rdr, err := gzip.NewReader(bytes.NewReader(rr.Body.Bytes()))
		if err != nil {
			t.Errorf("Gunzip failed: %s", err)
		} else {
			bbody, err := ioutil.ReadAll(rdr)
			if err != nil {
				t.Errorf("Gunzip read failed: %s", err)
			} else {
				body = string(bbody)
			}
		}
	} else {
		body = rr.Body.String()
	}
	if len(body) != 27 {
		t.Errorf("GET %s returned wrong decoded body length '%d', expected 27",
			encoding, len(body))
	}
	if body != expectedBody {
		t.Errorf("GET%s returned wrong body '%s'", encoding, body)
	}
}

func TestOpenStat(t *testing.T) {
	fh := &fileHandler{http.Dir(".")}
	_, _, err := fh.openAndStat(".")
	if err == nil {
		t.Errorf("openAndStat directory succeeded, should have failed")
	}
	_, _, err = fh.openAndStat("updog")
	if err == nil {
		t.Errorf("openAndStat nonexistent file succeeded, should have failed")
	}
}

func TestNoBrowse(t *testing.T) {
	fs := FileServer(http.Dir("./testdata/"))
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	fs.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Errorf("Directory browse succeeded")
	}
}

func TestLeadingSlash(t *testing.T) {
	fs := FileServer(http.Dir("./testdata/"))
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "file.txt", nil)
	fs.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("Missing leading / on HTTP path caused error")
	}
}

func Test404(t *testing.T) {
	fs := FileServer(http.Dir("./testdata/"))
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/nonexistent.txt", nil)
	fs.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Errorf("Directory browse succeeded")
	}
}

func TestGet(t *testing.T) {
	testGet(t, false, "zyxwvutsrqponmlkjihgfedcba\n")
}

func TestGzipGet(t *testing.T) {
	testGet(t, true, "abcdefghijklmnopqrstuvwxyz\n")
}

func TestConstHeaders(t *testing.T) {
	for _, header := range []string{
		acceptEncodingHeader,
		contentEncodingHeader,
		contentLengthHeader,
		rangeHeader,
		varyHeader,
	} {
		canonical := textproto.CanonicalMIMEHeaderKey(header)
		if header != canonical {
			t.Errorf("%s != %s", header, canonical)
		}
	}
}
