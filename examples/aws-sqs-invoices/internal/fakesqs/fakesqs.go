// Package fakesqs is a small stand-in for SQS, used by the example's tests.
package fakesqs

import (
	"context"
	"crypto/md5" //nolint:gosec // SQS's own message checksum
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Call is what the fake SQS saw of one request.
type Call struct {
	RequestURI    string
	Target        string
	ProxyAuth     string
	Authorization string
	Body          map[string]any
}

type message struct {
	id, body, receipt string
	inFlight          bool
}

// SQS answers SendMessage, ReceiveMessage and DeleteMessage the way SQS
// does over its JSON protocol. It also takes calls meant for a forward proxy,
// so a test can see what the app sends to Hotline.
type SQS struct {
	Addr     string
	mu       sync.Mutex
	calls    []Call
	messages []*message
	next     int
}

// Start listens on a free local port.
func Start() *SQS {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	f := &SQS{Addr: ln.Addr().String()}
	server := &http.Server{Handler: f, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(ln) }()
	return f
}

func (f *SQS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	target := r.Header.Get("X-Amz-Target")

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{
		RequestURI:    r.RequestURI,
		Target:        target,
		ProxyAuth:     r.Header.Get("Proxy-Authorization"),
		Authorization: r.Header.Get("Authorization"),
		Body:          in,
	})

	var out any
	switch strings.TrimPrefix(target, "AmazonSQS.") {
	case "SendMessage":
		f.next++
		m := &message{id: fmt.Sprintf("m-%d", f.next), body: in["MessageBody"].(string), receipt: fmt.Sprintf("r-%d", f.next)}
		f.messages = append(f.messages, m)
		out = map[string]any{"MessageId": m.id, "MD5OfMessageBody": md5Hex(m.body)}
	case "ReceiveMessage":
		var got []map[string]any
		for _, m := range f.messages {
			if m.inFlight {
				continue // stays hidden until deleted, like a long visibility timeout
			}
			m.inFlight = true
			got = append(got, map[string]any{"MessageId": m.id, "ReceiptHandle": m.receipt, "MD5OfBody": md5Hex(m.body), "Body": m.body})
		}
		// Like SQS, an empty receive has no Messages at all.
		reply := map[string]any{}
		if len(got) > 0 {
			reply["Messages"] = got
		}
		out = reply
	case "DeleteMessage":
		for i, m := range f.messages {
			if m.receipt == in["ReceiptHandle"] {
				f.messages = append(f.messages[:i], f.messages[i+1:]...)
				break
			}
		}
		out = map[string]any{}
	default:
		w.WriteHeader(http.StatusBadRequest)
		out = map[string]any{"__type": "com.amazonaws.sqs#InvalidAction", "message": "unknown action"}
	}
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		panic(err)
	}
}

// Calls lists every request so far.
func (f *SQS) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Waiting counts the messages not yet deleted.
func (f *SQS) Waiting() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // SQS's own message checksum
	return hex.EncodeToString(sum[:])
}
