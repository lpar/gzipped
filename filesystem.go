package gzipped

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type fileSystem struct {
	http.FileSystem
}

type FileSystem interface {
	http.FileSystem
	Exists(string) bool
}

type Dir string

// Exists tests whether a file with the specified name exists, resolved relative to the base directory.
func (d Dir) Exists(name string) bool {
	if filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator) {
		return false
	}
	dir := string(d)
	if dir == "" {
		dir = "."
	}
	fullName := filepath.Join(dir, filepath.FromSlash(path.Clean("/"+name)))
	_, err := os.Stat(fullName)
	if err != nil {
		return false
	}
	return true
}

// Open defers to http.Dir's Open so that gzipped.Dir implements http.FileSystem.
func (d Dir) Open(name string) (http.File, error) {
	return http.Dir(d).Open(name)
}
