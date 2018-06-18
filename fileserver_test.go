package gzipped

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

var trueHeaders = [...]string{
	"gzip",
	"*",
	"compress,gzip;q=0.1",
	"compress, * ;q=1",
	"deflate ;q=0.3, gzip ;q=0.7, x-foo",
}

var falseHeaders = [...]string{
	"gzip;q=0,*",
	"deflate; gzip ;q=0 , x-foo,*",
}

func TestGzipAcceptable(t *testing.T) {
	var req http.Request
	req.Header = make(http.Header)
	for _, ac := range trueHeaders {
		req.Header.Set("Accept-Encoding", ac)
		if !acceptable(&req, gzipEncoding) {
			t.Errorf("acceptable(%s, gzip) false, want true", ac)
		}
	}
	for _, ac := range falseHeaders {
		req.Header.Set("Accept-Encoding", ac)
		if acceptable(&req, gzipEncoding) {
			t.Errorf("acceptable(%s, gzip) true, want false", ac)
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
	// There should be no content-length on gzipped content.
	// See https://github.com/golang/go/issues/9987
	if len(clh) > 0 && withGzip {
		t.Errorf("Response had both Transfer-Encoding and Content-Length")
	}
	// Otherwise, check the content-length is correct.
	if len(clh) > 0 && !withGzip {
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
