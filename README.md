[![GoDoc](https://godoc.org/github.com/lpar/gzipped?status.svg)](https://godoc.org/github.com/lpar/gzipped)

# gzipped.FileServer

Drop-in replacement for golang http.FileServer which supports static content
compressed with gzip (including zopfli) or brotli.

This allows major bandwidth savings for CSS, JavaScript libraries, fonts, and
other static compressible web content. It also means you can compress the
content without significant runtime penalty.

## Example

Suppose `/var/www/assets/css` contains your style sheets, and you want to make them available as `/css/*.css`:

    package main
    
    import (
    	"log"
    	"net/http"
    
    	"github.com/lpar/gzipped"
    )
    
    func main() {
    	log.Fatal(http.ListenAndServe(":8080", http.StripPrefix("/css",
        gzipped.FileServer(gzipped.Dir("/var/www/assets/css")))))
    }
    // curl localhost:8080/css/styles.css


Using [httprouter](https://github.com/julienschmidt/httprouter)?

    router := httprouter.New()
    router.Handler("GET", "/css/*filepath", 
      gzipped.FileServer(gzipped.Dir("/var/www/assets/css"))))
    log.Fatal(http.ListenAndServe(":8080", router)

## Change history

In version 2.0, we require use of `gzipped.Dir`, a drop-in replacement for `http.Dir`. Our `gzipped.Dir` has the
additional feature of letting us check for the existence of files without opening them. This means we can scan
to see what encodings are available, then negotiate that list against the client's preferences, and then only (attempt
to) open and serve the correct file.

This change means we can let `github.com/kevinpollet/nego` handle the content negotiation, and remove the dependency
on gddo (godoc), which was pulling in 48 dependencies (see [#6](https://github.com/lpar/gzipped/issues/6)).

## Detail

For any given request at `/path/filename.ext`, if:

  1. There exists a file named `/path/filename.ext.(gz|br)` (starting from the 
     appropriate base directory), and
  2. the client will accept content compressed via the appropriate algorithm, and
  3. the file can be opened,

then the compressed file will be served as `/path/filename.ext`, with a
`Content-Encoding` header set so that the client transparently decompresses it.
Otherwise, the request is passed through and handled unchanged.

Unlike other similar code I found, this package has a license, parses 
Accept-Encoding headers properly, and has unit tests.

## Caveats

All requests are passed to Go's standard `http.ServeContent` method for
fulfilment. MIME type sniffing, accept ranges, content negotiation and other
tricky details are handled by that method.

It is up to you to ensure that your compressed and uncompressed resources are
kept in sync.

Directory browsing isn't supported. That includes remapping URLs ending in `/` to `index.html`, 
`index.htm`, `Welcome.html` or whatever -- if you want URLs remapped that way,
I suggest having your router do it, or using middleware, so that you have control
over the behavior. For example, to add support for `index.html` files in directories:

```go
func withIndexHTML(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			newpath := path.Join(r.URL.Path, "index.html")
			r.URL.Path = newpath
		}
		h.ServeHTTP(w, r)
	})
}
// ...

fs := withIndexHTML(gzipped.FileServer(http.Dir("/var/www")))
```

Or to add support for directory browsing:

```go
func withBrowsing(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.ServeFile(w, r, ".")
		}
	})
}
// ...

fs := withBrowsing(gzipped.FileServer(http.Dir("/var/www")))
```

## Related

 * You might consider precompressing your CSS with [minify](https://github.com/tdewolff/minify). 

 * If you want to get the best possible compression for clients which don't support brotli, use [zopfli](https://github.com/google/zopfli).

 * To compress your dynamically-generated HTML pages on the fly, I suggest [gziphandler](https://github.com/NYTimes/gziphandler).

