package gzipped

import (
	"bytes"
	"compress/gzip"
	"embed"
	"errors"
	fs2 "io/fs"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
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
	} {
		req.Header.Set("Accept-Encoding", info.hdr)
		negenc := nego.NegotiateContentEncoding(&req, preferredEncodings...)
		if negenc != info.expect {
			t.Errorf("server chose %s but we expected %s for header %s", negenc, info.expect, info.hdr)
		}
	}
}

func testGet(t *testing.T, f FileSystem, acceptGzip bool, urlPath string, expectedBody string) {
	fs := FileServer(f)
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

//go:embed testdata
var testData embed.FS

type TestCase struct {
	name string
	test func(t *testing.T)
}

func TestFileServer(t *testing.T) {
	tests := func(f FileSystem) []TestCase {
		return []TestCase{
			{
				name: "OpenStat",
				test: func(t *testing.T) {
					fh := &fileHandler{f}
					_, _, err := fh.openAndStat(".")
					if err == nil {
						t.Errorf("openAndStat directory succeeded, should have failed")
					}
					_, _, err = fh.openAndStat("updog")
					if err == nil {
						t.Errorf("openAndStat nonexistent file succeeded, should have failed")
					}
				},
			},
			{

				name: "NoBrowse",
				test: func(t *testing.T) {
					fs := FileServer(f)
					rr := httptest.NewRecorder()
					req, _ := http.NewRequest("GET", "/", nil)
					fs.ServeHTTP(rr, req)
					if rr.Code != 404 {
						t.Errorf("Directory browse succeeded")
					}
				},
			},
			{

				name: "LeadingSlash",
				test: func(t *testing.T) {
					fs := FileServer(f)
					rr := httptest.NewRecorder()
					req, _ := http.NewRequest("GET", "file.txt", nil)
					fs.ServeHTTP(rr, req)
					if rr.Code != 200 {
						t.Errorf("Missing leading / on HTTP path caused error")
					}
				},
			},
			{

				name: "404",
				test: func(t *testing.T) {
					fs := FileServer(f)
					rr := httptest.NewRecorder()
					req, _ := http.NewRequest("GET", "/nonexistent.txt", nil)
					fs.ServeHTTP(rr, req)
					if rr.Code != 404 {
						t.Errorf("Directory browse succeeded")
					}
				},
			},
			{

				name: "Get",
				test: func(t *testing.T) {
					testGet(t, f, false, "/file.txt", "zyxwvutsrqponmlkjihgfedcba\n")
				},
			},
			{

				name: "GzipGet",
				test: func(t *testing.T) {
					testGet(t, f, true, "/file.txt", "abcdefghijklmnopqrstuvwxyz\n")
				},
			},
			{

				name: "GetIdentity",
				test: func(t *testing.T) {
					testGet(t, f, false, "/file2.txt", "1234567890987654321\n")
				},
			},
			{

				name: "GzipGetIdentity",
				test: func(t *testing.T) {
					testGet(t, f, true, "/file2.txt", "1234567890987654321\n")
				},
			},
		}
	}

	sub, err := fs2.Sub(testData, "testdata")
	if err != nil {
		t.Fatal(err)
	}

	for name, fs := range map[string]FileSystem{"dir": Dir("./testdata/"), "fs": FS(sub)} {
		t.Run(name, func(t *testing.T) {
			for _, tt := range tests(fs) {
				t.Run(tt.name, tt.test)
			}
		})
	}
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

type trackedFileSystem struct {
	FileSystem
	statErrorFor string
	files        []*trackedFile
}

func (f *trackedFileSystem) Open(name string) (http.File, error) {
	file, err := f.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	tracked := &trackedFile{File: file}
	if name == f.statErrorFor {
		tracked.statErr = errors.New("stat failed")
	}
	f.files = append(f.files, tracked)
	return tracked, nil
}

type trackedFile struct {
	http.File
	statErr error
	closes  int
}

func (f *trackedFile) Stat() (os.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	return f.File.Stat()
}

func (f *trackedFile) Close() error {
	f.closes++
	return f.File.Close()
}

func TestFileServerClosesFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("plain"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "file.txt.gz"), 0700); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name         string
		path         string
		encoding     string
		statErrorFor string
		status       int
		opened       int
	}{
		{"directory", "/directory", "", "", 404, 1},
		{"directory_slash", "/directory/", "", "", 404, 1},
		{"stat_error", "/file.txt", "", "/file.txt", 404, 1},
		{"compressed_directory", "/file.txt", "gzip", "", 200, 2},
		{"compressed_stat_error", "/file.txt", "gzip", "/file.txt.gz", 200, 2},
		{"success", "/file.txt", "", "", 200, 1},
		{"not_found", "/missing", "", "", 404, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fs := &trackedFileSystem{FileSystem: Dir(root), statErrorFor: tt.statErrorFor}
			defer func() {
				for _, file := range fs.files {
					if file.closes == 0 {
						file.File.Close()
					}
				}
			}()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Header.Set("Accept-Encoding", tt.encoding)
			response := httptest.NewRecorder()
			FileServer(fs).ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Errorf("status = %d, want %d", response.Code, tt.status)
			}
			if len(fs.files) != tt.opened {
				t.Errorf("opened %d files, want %d", len(fs.files), tt.opened)
			}
			for i, file := range fs.files {
				if file.closes != 1 {
					t.Errorf("file %d closed %d times, want 1", i, file.closes)
				}
			}
			if tt.status == 200 && response.Body.String() != "plain" {
				t.Errorf("body = %q, want plain", response.Body.String())
			}
		})
	}
}
