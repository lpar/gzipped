package gzipped

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/kevinpollet/nego"
)

// Encoding represents an Accept-Encoding. All of these fields are pre-populated
// in the supportedEncodings variable, except the clientPreference which is updated
// (by copying a value from supportedEncodings) when examining client headers.
type encoding struct {
	name             string  // the encoding name
	extension        string  // the file extension (including a leading dot)
	clientPreference float64 // the client's preference
	serverPreference int     // the server's preference
}

// Helper type to sort encodings, using clientPreference first, and then
// serverPreference as a tie breaker. This sorts in *DESCENDING* order, rather
// than the usual ascending order.
type encodingByPreference []encoding

// Implement the sort.Interface interface
func (e encodingByPreference) Len() int { return len(e) }
func (e encodingByPreference) Less(i, j int) bool {
	if e[i].clientPreference == e[j].clientPreference {
		return e[i].serverPreference > e[j].serverPreference
	}
	return e[i].clientPreference > e[j].clientPreference
}
func (e encodingByPreference) Swap(i, j int) { e[i], e[j] = e[j], e[i] }

// Supported encodings. Higher server preference means the encoding will be when
// the client doesn't have an explicit preference.
var supportedEncodings = [...]encoding{
	{
		name:             "gzip",
		extension:        ".gz",
		serverPreference: 1,
	},
	{
		name:             "br",
		extension:        ".br",
		serverPreference: 2,
	},
}

// List of encodings we would prefer to use, in order of preference, best first.
var preferredEncodings = []string{"br", "gzip", "identity"}

// File extension to use for different encodings.
func extensionForEncoding(encname string) string {
	switch encname {
	case "gzip":
		return ".gz"
	case "br":
		return ".br"
	case "identity":
		return ""
	}
	return ""
}

// Function to negotiate the best content encoding
// Pulled out here so we have the option of overriding nego's behavior and so we can test
func negotiate(r *http.Request, available []string) string {
	return nego.NegotiateContentEncoding(r, available...)
}

type fileHandler struct {
	root FileSystem
}

// FileServer is a drop-in replacement for Go's standard http.FileServer
// which adds support for static resources precompressed with gzip, at
// the cost of removing the support for directory browsing.
//
// If file filename.ext has a compressed version filename.ext.gz alongside
// it, if the client indicates that it accepts gzip-compressed data, and
// if the .gz file can be opened, then the compressed version of the file
// will be sent to the client. Otherwise the request is passed on to
// http.ServeContent, and the raw (uncompressed) version is used.
//
// It is up to you to ensure that the compressed and uncompressed versions
// of files match and have sensible timestamps.
//
// Compressed or not, requests are fulfilled using http.ServeContent, and
// details like accept ranges and content-type sniffing are handled by that
// method.
func FileServer(root FileSystem) http.Handler {
	return &fileHandler{root}
}

func (f *fileHandler) openAndStat(path string) (http.File, os.FileInfo, error) {
	file, err := f.root.Open(path)
	var info os.FileInfo
	// This slightly weird variable reuse is so we can get 100% test coverage
	// without having to come up with a test file that can be opened, yet
	// fails to stat.
	if err == nil {
		info, err = file.Stat()
	}
	if err != nil {
		return file, nil, err
	}
	if info.IsDir() {
		return file, nil, fmt.Errorf("%s is directory", path)
	}
	return file, info, nil
}

const (
	acceptEncodingHeader  = "Accept-Encoding"
	contentEncodingHeader = "Content-Encoding"
	contentLengthHeader   = "Content-Length"
	rangeHeader           = "Range"
	varyHeader            = "Vary"
)

// Find the best file to serve based on the client's Accept-Encoding, and which
// files actually exist on the filesystem. If no file was found that can satisfy
// the request, the error field will be non-nil.
func (f *fileHandler) findBestFile(w http.ResponseWriter, r *http.Request, fpath string) (http.File, os.FileInfo, error) {
	ae := r.Header.Get(acceptEncodingHeader)
	if ae == "" {
		return f.openAndStat(fpath)
	}
	// Got an accept header? See what possible encodings we can send by looking for files
	var available []string
	for _, posenc := range preferredEncodings {
		ext := extensionForEncoding(posenc)
		fname := fpath + ext
		if f.root.Exists(fname) {
			available = append(available, posenc)
			fmt.Printf("%s (%s) available\n", fname, posenc)
		} else {
			fmt.Printf("%s (%s) not found\n", fname, posenc)
		}
	}
	if len(available) == 0 {
		return f.openAndStat(fpath)
	}
	// Carry out standard HTTP negotiation
	negenc := negotiate(r, available)
	if negenc == "" {
		// If we fail to negotiate anything, again try the base file
		return f.openAndStat(fpath)
	}
	ext := extensionForEncoding(negenc)
	if file, info, err := f.openAndStat(fpath + ext); err == nil {
		wHeader := w.Header()
		wHeader[contentEncodingHeader] = []string{negenc}
		wHeader.Add(varyHeader, acceptEncodingHeader)

		if len(r.Header[rangeHeader]) == 0 {
			// If not a range request then we can easily set the content length which the
			// Go standard library does not do if "Content-Encoding" is set.
			wHeader[contentLengthHeader] = []string{strconv.FormatInt(info.Size(), 10)}
		}
		return file, info, nil
	}

	// If all else failed, fall back to base file once again
	return f.openAndStat(fpath)
}

func (f *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
		r.URL.Path = upath
	}
	fpath := path.Clean(upath)
	if strings.HasSuffix(fpath, "/") {
		// If you wanted to put back directory browsing support, this is
		// where you'd do it.
		http.NotFound(w, r)
		return
	}

	// Find the best acceptable file, including trying uncompressed
	if file, info, err := f.findBestFile(w, r, fpath); err == nil {
		http.ServeContent(w, r, fpath, info.ModTime(), file)
		file.Close()
		return
	}

	// Doesn't exist, compressed or uncompressed
	http.NotFound(w, r)
}
