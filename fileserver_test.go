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

	"github.com/kevinpollet/nego"
)

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
	}{
		req.Header.Set("Accept-Encoding", info.hdr)
		negenc := nego.NegotiateContentEncoding(&req, preferredEncodings...)
		if negenc != info.expect {
			t.Errorf("server chose %s but we expected %s for header %s", negenc, info.expect, info.hdr)
		}
	}
}

func testGet(t *testing.T, acceptGzip bool, urlPath string, expectedBody string) {
	fs := FileServer(Dir("./testdata/"))
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", urlPath, nil)
	if acceptGzip {
		req.Header.Set("Accept-Encoding", "gzip,*")
	}
	fs.ServeHTTP(rr, req)
	h := rr.Header()

	// Check the content-length is correct.
	clh := h["Content-Length"]
	if len(clh) > 0 {
		byts, err := strconv.Atoi(clh[0])
		if err != nil {
			t.Errorf("Invalid Content-Length on response: '%s'", clh[0])
		}
		n := rr.Body.Len()
		if n != byts {
			t.Errorf("GET expected %d byts, got %d", byts, n)
		}
	}

	// Check the body content is correct.
	ce := h["Content-Encoding"]
	var body string
	if len(ce) > 0 {
		if ce[0] == "gzip" {
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
			t.Errorf("Invalid Content-Encoding in response: '%s'", ce[0])
		}
	} else {
		body = rr.Body.String()
	}
	if len(body) != len(expectedBody) {
		t.Errorf("GET (acceptGzip=%v) returned wrong decoded body length %d, expected %d",
			acceptGzip, len(body), len(expectedBody))
	}
	if body != expectedBody {
		t.Errorf("GET (acceptGzip=%v) returned wrong body '%s'", acceptGzip, body)
	}
}

func TestOpenStat(t *testing.T) {
	fh := &fileHandler{Dir(".")}
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
	fs := FileServer(Dir("./testdata/"))
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	fs.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Errorf("Directory browse succeeded")
	}
}

func TestLeadingSlash(t *testing.T) {
	fs := FileServer(Dir("./testdata/"))
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "file.txt", nil)
	fs.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("Missing leading / on HTTP path caused error")
	}
}

func Test404(t *testing.T) {
	fs := FileServer(Dir("./testdata/"))
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/nonexistent.txt", nil)
	fs.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Errorf("Directory browse succeeded")
	}
}

func TestGet(t *testing.T) {
	testGet(t, false, "/file.txt", "zyxwvutsrqponmlkjihgfedcba\n")
}

func TestGzipGet(t *testing.T) {
	testGet(t, true, "/file.txt", "abcdefghijklmnopqrstuvwxyz\n")
}

func TestGetIdentity(t *testing.T) {
	testGet(t, false, "/file2.txt", "1234567890987654321\n")
}

func TestGzipGetIdentity(t *testing.T) {
	testGet(t, true, "/file2.txt", "1234567890987654321\n")
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
