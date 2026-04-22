// Command bonsai-ollama-proxy sits in front of Ollama: it reverse-proxies almost
// everything to a real ollama serve, but routes Bonsai Q1_0 requests to Prism's
// llama-server (OpenAI-compatible API) because stock Ollama cannot load GGML Q1_0.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	listen := env("BONSAI_PROXY_LISTEN", "127.0.0.1:11434")
	backend := env("BONSAI_OLLAMA_BACKEND", "http://127.0.0.1:11435")
	llamaPort := env("BONSAI_LLAMA_PORT", "9988")
	llamaHost := env("BONSAI_LLAMA_HOST", "127.0.0.1")

	repoRoot := os.Getenv("BONSAI_REPO_ROOT")
	if repoRoot == "" {
		exe, _ := os.Executable()
		repoRoot = filepath.Clean(filepath.Join(filepath.Dir(exe), "..", ".."))
	}
	prismDir := os.Getenv("BONSAI_PRISM_LIB_DIR")
	if prismDir == "" {
		prismDir = filepath.Join(repoRoot, "vendor", "prism-llama", "llama-prism-b8846-d104cf1")
	}
	gguf := os.Getenv("BONSAI_GGUF")
	if gguf == "" {
		gguf = filepath.Join(repoRoot, "models", "bonsai-1.7b", "Bonsai-1.7B-Q1_0.gguf")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	llamaSrv := filepath.Join(prismDir, "llama-server")
	if _, err := os.Stat(llamaSrv); err != nil {
		log.Fatalf("llama-server not found at %s: %v", llamaSrv, err)
	}
	if _, err := os.Stat(gguf); err != nil {
		log.Fatalf("GGUF not found at %s: %v", gguf, err)
	}

	llamaCmd := exec.CommandContext(ctx, llamaSrv,
		"-m", gguf,
		"--host", llamaHost,
		"--port", llamaPort,
	)
	ldPath := prismDir
	if prev := os.Getenv("LD_LIBRARY_PATH"); prev != "" {
		ldPath = prismDir + string(os.PathListSeparator) + prev
	}
	llamaCmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+ldPath)
	llamaCmd.Dir = prismDir
	llamaCmd.Stderr = os.Stderr
	llamaCmd.Stdout = os.Stdout
	if err := llamaCmd.Start(); err != nil {
		log.Fatalf("start llama-server: %v", err)
	}
	go func() { _ = llamaCmd.Wait() }()

	llamaBase := fmt.Sprintf("http://%s:%s", llamaHost, llamaPort)
	if err := waitTCP(ctx, net.JoinHostPort(llamaHost, llamaPort), 60*time.Second); err != nil {
		log.Fatalf("llama-server did not listen: %v", err)
	}
	log.Printf("llama-server ready at %s", llamaBase)

	backendURL, err := url.Parse(backend)
	if err != nil {
		log.Fatalf("backend URL: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(backendURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, e.Error(), http.StatusBadGateway)
	}

	srv := &http.Server{
		Addr: listen,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldHandleBonsai(r) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				_ = r.Body.Close()
				var probe map[string]any
				if json.Unmarshal(body, &probe) != nil {
					r.Body = io.NopCloser(bytes.NewReader(body))
					proxy.ServeHTTP(w, r)
					return
				}
				model, _ := probe["model"].(string)
				if !isBonsaiModel(model) {
					r.Body = io.NopCloser(bytes.NewReader(body))
					proxy.ServeHTTP(w, r)
					return
				}
				switch r.URL.Path {
				case "/api/chat":
					handleBonsaiChat(w, r, body, llamaBase, model)
				case "/api/generate":
					handleBonsaiGenerate(w, r, body, llamaBase, model)
				default:
					r.Body = io.NopCloser(bytes.NewReader(body))
					proxy.ServeHTTP(w, r)
				}
				return
			}
			proxy.ServeHTTP(w, r)
		}),
	}

	go func() {
		<-ctx.Done()
		shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shCancel()
		_ = srv.Shutdown(shCtx)
	}()

	log.Printf("bonsai-ollama-proxy listening on %s -> ollama %s", listen, backend)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func shouldHandleBonsai(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/chat", "/api/generate":
		return true
	default:
		return false
	}
}

func isBonsaiModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	m = strings.TrimSuffix(m, ":latest")
	return strings.Contains(m, "bonsai-1.7b")
}

func waitTCP(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

// --- /api/generate ---

func handleBonsaiGenerate(w http.ResponseWriter, r *http.Request, body []byte, llamaBase, model string) {
	var req struct {
		Prompt   string         `json:"prompt"`
		Stream   bool           `json:"stream"`
		Options  map[string]any `json:"options"`
		Template string         `json:"template"`
		System   string         `json:"system"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	maxTok := intFromOptions(req.Options, "num_predict", 512)
	temp := floatFromOptions(req.Options, "temperature", 0.5)
	topP := floatFromOptions(req.Options, "top_p", 0.85)

	messages := buildChatMessages(req.System, req.Prompt)
	openAI := openAIChatPayload(messages, maxTok, temp, topP, req.Stream)

	if req.Stream {
		if err := pipeLlamaOpenAIStreamToOllama(r.Context(), w, llamaBase, openAI, model, ollamaStreamGenerate); err != nil {
			log.Printf("bonsai generate stream: %v", err)
		}
		return
	}

	text, err := postOpenAIChat(llamaBase, openAI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	resp := map[string]any{
		"model":      model,
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"response":   text,
		"done":       true,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// --- /api/chat ---

func handleBonsaiChat(w http.ResponseWriter, r *http.Request, body []byte, llamaBase, model string) {
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream  bool           `json:"stream"`
		Options map[string]any `json:"options"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	maxTok := intFromOptions(req.Options, "num_predict", 512)
	temp := floatFromOptions(req.Options, "temperature", 0.5)
	topP := floatFromOptions(req.Options, "top_p", 0.85)

	msgs := make([]map[string]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "" {
			continue
		}
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}

	openAI := openAIChatPayload(msgs, maxTok, temp, topP, req.Stream)

	if req.Stream {
		if err := pipeLlamaOpenAIStreamToOllama(r.Context(), w, llamaBase, openAI, model, ollamaStreamChat); err != nil {
			log.Printf("bonsai chat stream: %v", err)
		}
		return
	}

	text, err := postOpenAIChat(llamaBase, openAI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	resp := map[string]any{
		"model":      model,
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"message": map[string]any{
			"role":    "assistant",
			"content": text,
		},
		"done": true,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func buildChatMessages(system, prompt string) []map[string]string {
	messages := []map[string]string{}
	if system != "" {
		messages = append(messages, map[string]string{"role": "system", "content": system})
	}
	messages = append(messages, map[string]string{"role": "user", "content": prompt})
	return messages
}

func openAIChatPayload(messages []map[string]string, maxTok int, temp, topP float64, stream bool) map[string]any {
	return map[string]any{
		"model":       "bonsai",
		"messages":    messages,
		"max_tokens":  maxTok,
		"temperature": temp,
		"top_p":       topP,
		"stream":      stream,
	}
}

var (
	httpClient = &http.Client{Timeout: 10 * time.Minute}
	// streamClient has no global timeout so response bodies can be read incrementally.
	streamClient = &http.Client{Timeout: 0}
)

func postOpenAIChat(llamaBase string, payload map[string]any) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	u := strings.TrimSuffix(llamaBase, "/") + "/v1/chat/completions"
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	rb, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama-server %d: %s", res.StatusCode, string(rb))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rb, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("no choices in llama response")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

type ollamaStreamFormat int

const (
	ollamaStreamChat ollamaStreamFormat = iota
	ollamaStreamGenerate
)

// pipeLlamaOpenAIStreamToOllama POSTs a streaming chat-completions request to llama-server,
// reads the OpenAI SSE response, and writes Ollama NDJSON (chat or generate shape).
func pipeLlamaOpenAIStreamToOllama(ctx context.Context, w http.ResponseWriter, llamaBase string, openAIPayload map[string]any, model string, streamKind ollamaStreamFormat) error {
	b, err := json.Marshal(openAIPayload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}
	u := strings.TrimSuffix(llamaBase, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	res, err := streamClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(res.Body)
		http.Error(w, fmt.Sprintf("llama-server %d: %s", res.StatusCode, string(rb)), http.StatusBadGateway)
		return fmt.Errorf("status %d", res.StatusCode)
	}

	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming requires Flusher", http.StatusInternalServerError)
		return fmt.Errorf("no flusher")
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	created := time.Now().UTC().Format(time.RFC3339Nano)

	if streamKind == ollamaStreamChat {
		if err := enc.Encode(map[string]any{
			"model": model, "created_at": created,
			"message": map[string]any{"role": "assistant", "content": ""},
			"done": false,
		}); err != nil {
			return err
		}
		fl.Flush()
	}

	var evalCount int
	// One Ollama NDJSON line per llama-server SSE delta (same granularity as backend tokens /
	// sub-tokens). Flush immediately so the client receives each chunk as soon as it arrives.
	if err := forEachOpenAISSEDataLine(res.Body, func(delta string, finishReason string) error {
		if delta != "" {
			evalCount++
			switch streamKind {
			case ollamaStreamChat:
				if err := enc.Encode(map[string]any{
					"model": model, "created_at": created,
					"message": map[string]any{"role": "assistant", "content": delta},
					"done": false,
				}); err != nil {
					return err
				}
			case ollamaStreamGenerate:
				if err := enc.Encode(map[string]any{
					"model": model, "created_at": created,
					"response": delta,
					"done":     false,
				}); err != nil {
					return err
				}
			}
			fl.Flush()
		}
		if finishReason != "" {
			return errStopSSE
		}
		return nil
	}); err != nil && !errors.Is(err, errStopSSE) {
		return err
	}

	var finalErr error
	switch streamKind {
	case ollamaStreamChat:
		finalErr = enc.Encode(map[string]any{
			"model":          model,
			"created_at":     created,
			"message":        map[string]any{"role": "assistant", "content": ""},
			"done":           true,
			"done_reason":    "stop",
			"eval_count":     evalCount,
			"total_duration": 0,
		})
	case ollamaStreamGenerate:
		finalErr = enc.Encode(map[string]any{
			"model":          model,
			"created_at":     created,
			"response":       "",
			"done":           true,
			"done_reason":    "stop",
			"eval_count":     evalCount,
			"total_duration": 0,
		})
	default:
		return nil
	}
	if finalErr != nil {
		return finalErr
	}
	fl.Flush()
	return nil
}

var errStopSSE = errors.New("sse done")

// forEachOpenAISSEDataLine scans an OpenAI-style text/event-stream body and invokes fn
// for each JSON payload line with non-empty text delta and/or a non-empty finish_reason.
func forEachOpenAISSEDataLine(r io.Reader, fn func(delta string, finishReason string) error) error {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 512*1024)
	sc.Buffer(buf, 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content *string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]
		delta := ""
		if ch.Delta.Content != nil {
			delta = *ch.Delta.Content
		}
		fr := ""
		if ch.FinishReason != nil {
			fr = *ch.FinishReason
		}
		if err := fn(delta, fr); err != nil {
			return err
		}
	}
	return sc.Err()
}

func intFromOptions(o map[string]any, key string, def int) int {
	if o == nil {
		return def
	}
	v, ok := o[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return def
	}
}

func floatFromOptions(o map[string]any, key string, def float64) float64 {
	if o == nil {
		return def
	}
	v, ok := o[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		return t
	default:
		return def
	}
}
