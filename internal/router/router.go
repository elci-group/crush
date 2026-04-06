package router

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type Config struct {
	LocalPool string
	CloudAPI  string
}

type Router struct {
	config Config
	client *http.Client
}

func New(cfg Config) *Router {
	return &Router{
		config: cfg,
		client: &http.Client{},
	}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost || !strings.HasSuffix(req.URL.Path, "/v1/chat/completions") {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var lastMessage string
	if len(payload.Messages) > 0 {
		lastMessage = strings.ToLower(payload.Messages[len(payload.Messages)-1].Content)
	}

	// Simple Keyword Router
	localKeywords := []string{"license", "boilerplate", "format", "docstring", "readme"}
	useLocal := false
	for _, k := range localKeywords {
		if strings.Contains(lastMessage, k) {
			useLocal = true
			break
		}
	}

	targetURL := r.config.CloudAPI
	if useLocal {
		targetURL = r.config.LocalPool
	}

	// Ensure target URL correctly appends the path if needed
	if !strings.HasSuffix(targetURL, "/v1/chat/completions") {
		targetURL = strings.TrimRight(targetURL, "/") + req.URL.Path
	}

	slog.Info("Routing request", "target", targetURL, "use_local", useLocal)

	proxyReq, err := http.NewRequestWithContext(req.Context(), req.Method, targetURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for k, vv := range req.Header {
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}

	resp, err := r.client.Do(proxyReq)
	if err != nil {
		slog.Error("Proxy request failed", "error", err, "target", targetURL)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	flusher, isFlusher := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if isFlusher {
				flusher.Flush()
			}
		}
		if err != nil {
			if err != io.EOF {
				slog.Error("Failed to write response", "error", err)
			}
			break
		}
	}
}
