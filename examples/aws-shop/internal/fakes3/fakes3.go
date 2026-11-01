// Package fakes3 is a small stand-in for S3, used by the example's tests.
package fakes3

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Call is what the fake S3 saw of one request.
type Call struct {
	Method        string
	RequestURI    string
	Host          string
	ProxyAuth     string
	Authorization string
	ContentType   string
	// Chunked says the body came in AWS's chunks, with its checksum after
	// the body; ChecksumOK says that checksum matched.
	Chunked    bool
	ChecksumOK bool
}

type object struct {
	data     []byte
	modified time.Time
}

// S3 answers PutObject, GetObject, ListObjectsV2 and DeleteObjects for one
// bucket, the way S3 does. It also takes calls meant for a forward proxy,
// so a test can see what the app sends to Hotline; a call for another host
// goes to Other, such as a fake SQS.
type S3 struct {
	Addr   string
	Bucket string
	Other  http.Handler
	mu     sync.Mutex
	calls  []Call
	files  map[string]object
	now    time.Time
}

// Start listens on a free local port, for the bucket.
func Start(bucket string, other http.Handler) *S3 {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	f := &S3{Addr: ln.Addr().String(), Bucket: bucket, Other: other, files: map[string]object{}, now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	server := &http.Server{Handler: f, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(ln) }()
	return f
}

// Put stores a file, as if it was stored at the given time.
func (f *S3) Put(key string, data []byte, modified time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[key] = object{data: data, modified: modified}
}

// File is a stored file's bytes, nil when there is none.
func (f *S3) File(key string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[key].data
}

// Keys lists the stored files.
func (f *S3) Keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.files))
	for k := range f.files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Calls lists every request so far.
func (f *S3) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

func (f *S3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, _ := strings.Cut(r.Host, ":")
	path := strings.TrimPrefix(r.URL.Path, "/")
	switch {
	case strings.HasPrefix(host, f.Bucket+"."):
	case strings.HasPrefix(path, f.Bucket+"/") || path == f.Bucket:
		// A local S3 is called with the bucket in the path.
		path = strings.TrimPrefix(strings.TrimPrefix(path, f.Bucket), "/")
	case f.Other != nil:
		f.Other.ServeHTTP(w, r)
		return
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	body, chunked, checksumOK := readBody(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{
		Method: r.Method, RequestURI: r.RequestURI, Host: r.Host,
		ProxyAuth: r.Header.Get("Proxy-Authorization"), Authorization: r.Header.Get("Authorization"),
		ContentType: r.Header.Get("Content-Type"), Chunked: chunked, ChecksumOK: checksumOK,
	})
	query := r.URL.Query()
	switch {
	case r.Method == http.MethodGet && path == "" && query.Get("list-type") == "2":
		f.list(w, query.Get("prefix"))
	case r.Method == http.MethodPost && path == "" && query.Has("delete"):
		f.delete(w, body)
	case r.Method == http.MethodPut && path != "":
		f.files[path] = object{data: body, modified: f.now}
		w.Header().Set("ETag", `"e-1"`)
	case r.Method == http.MethodGet && path != "":
		o, ok := f.files[path]
		if !ok {
			writeXML(w, http.StatusNotFound, s3Error{Code: "NoSuchKey", Message: "The specified key does not exist.", RequestID: "req-1"})
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(o.data)
	default:
		writeXML(w, http.StatusBadRequest, s3Error{Code: "InvalidRequest", Message: "not in the fake", RequestID: "req-1"})
	}
}

type s3Error struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string
	Message   string
	RequestID string `xml:"RequestId"`
}

type listResult struct {
	XMLName     xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListBucketResult"`
	Name        string
	Prefix      string
	KeyCount    int
	MaxKeys     int
	IsTruncated bool
	Contents    []listed
}

type listed struct {
	Key          string
	LastModified string
	Size         int
	StorageClass string
}

func (f *S3) list(w http.ResponseWriter, prefix string) {
	out := listResult{Name: f.Bucket, Prefix: prefix, MaxKeys: 1000}
	keys := make([]string, 0, len(f.files))
	for k := range f.files {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		o := f.files[k]
		out.Contents = append(out.Contents, listed{Key: k, LastModified: o.modified.Format("2006-01-02T15:04:05.000Z"), Size: len(o.data), StorageClass: "STANDARD"})
	}
	out.KeyCount = len(out.Contents)
	writeXML(w, http.StatusOK, out)
}

type deleteRequest struct {
	Quiet   bool
	Objects []struct {
		Key string
	} `xml:"Object"`
}

type deleteResult struct {
	XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ DeleteResult"`
	Deleted []struct {
		Key string
	}
}

func (f *S3) delete(w http.ResponseWriter, body []byte) {
	var in deleteRequest
	if err := xml.Unmarshal(body, &in); err != nil {
		writeXML(w, http.StatusBadRequest, s3Error{Code: "MalformedXML", Message: err.Error(), RequestID: "req-1"})
		return
	}
	var out deleteResult
	for _, o := range in.Objects {
		delete(f.files, o.Key)
		out.Deleted = append(out.Deleted, struct{ Key string }{o.Key})
	}
	writeXML(w, http.StatusOK, out)
}

func writeXML(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("x-amz-request-id", "req-1")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, xml.Header)
	if err := xml.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}

// readBody reads a call's body. A body in AWS's chunks (aws-chunked) is
// put back together, and the CRC32 checksum sent after it is checked.
func readBody(r *http.Request) (body []byte, chunked, checksumOK bool) {
	data, _ := io.ReadAll(r.Body)
	if !strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") {
		return data, false, false
	}
	body, trailer, err := unchunk(data)
	if err != nil {
		return nil, true, false
	}
	sum := make([]byte, 4)
	binary.BigEndian.PutUint32(sum, crc32.ChecksumIEEE(body))
	return body, true, trailer["x-amz-checksum-crc32"] == base64.StdEncoding.EncodeToString(sum)
}

var errChunks = errors.New("broken aws-chunked body")

// unchunk reads aws-chunked: each chunk is its size in hex (and, when
// signed, ";chunk-signature=..."), a line break, the bytes and a line
// break; a chunk of size 0 ends the body, followed by trailer lines.
func unchunk(data []byte) ([]byte, map[string]string, error) {
	in := bufio.NewReader(bytes.NewReader(data))
	var body []byte
	for {
		line, err := in.ReadString('\n')
		if err != nil {
			return nil, nil, errChunks
		}
		sizeText, _, _ := strings.Cut(strings.TrimSpace(line), ";")
		size, err := strconv.ParseInt(sizeText, 16, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: chunk size %q", errChunks, sizeText)
		}
		if size == 0 {
			break
		}
		chunk := make([]byte, size+2)
		if _, err := io.ReadFull(in, chunk); err != nil {
			return nil, nil, errChunks
		}
		body = append(body, chunk[:size]...)
	}
	// The trailer lines end the body; a last line break may be missing.
	trailer := map[string]string{}
	rest, _ := io.ReadAll(in)
	for _, line := range strings.Split(string(rest), "\n") {
		if name, value, found := strings.Cut(strings.TrimSpace(line), ":"); found {
			trailer[strings.ToLower(name)] = value
		}
	}
	return body, trailer, nil
}
