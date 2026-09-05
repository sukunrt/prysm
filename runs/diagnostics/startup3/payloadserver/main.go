package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

//go:embed response.json
var result json.RawMessage

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
}

func main() {
	listen := flag.String("listen", "127.0.0.1:18561", "TCP address")
	responseBytes := flag.Int("response-bytes", 2060, "padded JSON response size")
	flag.Parse()
	var compact bytes.Buffer
	if err := json.Compact(&compact, result); err != nil {
		log.Fatal(err)
	}
	result = compact.Bytes()
	if *responseBytes < len(result) {
		log.Fatal("invalid embedded response or response size")
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(map[string]any{"event": "ready", "address": ln.Addr().String(), "pid": os.Getpid()})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		defer r.Body.Close()
		body, readErr := io.ReadAll(r.Body)
		var req request
		decodeErr := json.Unmarshal(body, &req)
		if readErr != nil || decodeErr != nil || req.Method != "engine_getPayloadV6" {
			http.Error(w, "invalid engine_getPayloadV6 request", http.StatusBadRequest)
			return
		}
		response := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":%s}`, req.ID, result))
		if len(response) > *responseBytes {
			http.Error(w, "configured response size too small", http.StatusInternalServerError)
			return
		}
		padding := *responseBytes - len(response)
		response = append(response, bytes.Repeat([]byte{' '}, padding)...)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", fmt.Sprint(len(response)))
		w.WriteHeader(http.StatusOK)
		written, writeErr := w.Write(response)
		flushErr := http.NewResponseController(w).Flush()
		_ = enc.Encode(map[string]any{
			"event": "response_flushed", "id": string(req.ID), "method": req.Method,
			"bytes": written, "duration_us": time.Since(started).Microseconds(),
			"write_error": fmt.Sprint(writeErr), "flush_error": fmt.Sprint(flushErr),
			"wall_unix_nano": time.Now().UnixNano(),
		})
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
