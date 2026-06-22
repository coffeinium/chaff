package dpi

import (
	"bytes"
	"strings"
)

var httpMethods = [][]byte{
	[]byte("GET "), []byte("POST "), []byte("HEAD "), []byte("PUT "),
	[]byte("DELETE "), []byte("OPTIONS "), []byte("PATCH "),
	[]byte("CONNECT "), []byte("TRACE "),
}

func HTTPHost(b []byte) (string, bool) {
	if !looksHTTP(b) {
		return "", false
	}
	for {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		line := bytes.TrimRight(b[:i], "\r")
		b = b[i+1:]
		if len(line) == 0 {
			break
		}
		k := bytes.IndexByte(line, ':')
		if k <= 0 {
			continue
		}
		if strings.EqualFold(string(bytes.TrimSpace(line[:k])), "host") {
			return strings.TrimSpace(string(line[k+1:])), true
		}
	}
	return "", false
}

func looksHTTP(b []byte) bool {
	for _, m := range httpMethods {
		if bytes.HasPrefix(b, m) {
			return true
		}
	}
	return false
}
