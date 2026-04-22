// /usr/bin/env go run ./cmd/bench-llama-tokens
//
// Measure Bonsai token timings via Prism llama-server OpenAI API (non-stream).
// Built binary: ./bin/bench_llama-tokens (see bin/run.sh / bin/setup.sh).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	decodePrompt = "You are benchmarking. Output continuous prose only, no lists or headings. " +
		"Explain how rivers shape landscapes, erosion, deltas, and floodplains in detail."
)

var prefillPrompt = "Summarize the following article in one sentence under 30 words:\n\n" +
	strings.Repeat("Lorem ipsum dolor sit amet. ", 120)

func postChat(client *http.Client, base, prompt string, maxTok int, temp float64) (map[string]any, error) {
	body, err := json.Marshal(map[string]any{
		"model":       "bonsai",
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  maxTok,
		"temperature": temp,
		"stream":      false,
	})
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(base, "/") + "/v1/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", res.StatusCode, string(raw))
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func runOnce(client *http.Client, base, prompt string, maxTok int, temp float64) (map[string]any, error) {
	t0 := time.Now()
	d, err := postChat(client, base, prompt, maxTok, temp)
	if err != nil {
		return nil, err
	}
	d["wall_s"] = time.Since(t0).Seconds()
	return d, nil
}

func summarize(xs []float64) map[string]any {
	var pos []float64
	for _, x := range xs {
		if x > 0 {
			pos = append(pos, x)
		}
	}
	if len(pos) == 0 {
		return map[string]any{}
	}
	mean := 0.0
	for _, x := range pos {
		mean += x
	}
	mean /= float64(len(pos))
	var stdev float64
	if len(pos) > 1 {
		var ss float64
		for _, x := range pos {
			d := x - mean
			ss += d * d
		}
		stdev = math.Sqrt(ss / float64(len(pos)-1))
	}
	minV, maxV := pos[0], pos[0]
	for _, x := range pos[1:] {
		if x < minV {
			minV = x
		}
		if x > maxV {
			maxV = x
		}
	}
	return map[string]any{
		"n":     len(pos),
		"mean":  mean,
		"stdev": stdev,
		"min":   minV,
		"max":   maxV,
	}
}

func getFloat(m map[string]any, k string) (float64, bool) {
	v, ok := m[k]
	if !ok || v == nil {
		return 0, false
	}
	if x, ok := v.(float64); ok {
		return x, true
	}
	return 0, false
}

func getInt(m map[string]any, k string) (int, bool) {
	v, ok := m[k]
	if !ok || v == nil {
		return 0, false
	}
	if x, ok := v.(float64); ok {
		return int(x), true
	}
	return 0, false
}

func main() {
	base := flag.String("base", getenvDefault("BONSAI_LLAMA_URL", "http://127.0.0.1:9988"), "llama-server base URL")
	runs := flag.Int("runs", 5, "iterations per scenario")
	warmup := flag.Int("warmup", 1, "warmup requests (discarded)")
	prefillWarmup := flag.Int("prefill-warmup", 3, "prefill warmup repeats before measuring")
	jsonOut := flag.Bool("json", false, "print machine-readable summary only")
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Minute}

	if _, err := postChat(client, *base, "ping", 2, 0); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot reach %s: %v\n", *base, err)
		fmt.Fprintln(os.Stderr, "Start the stack first: ./bin/run.sh")
		os.Exit(1)
	}

	for i := 0; i < *warmup; i++ {
		_, _ = postChat(client, *base, "Say OK.", 4, 0.1)
	}

	var decodeRates []float64
	var promptRates []float64
	var predictedNs []int
	var walls []float64

	for i := 0; i < *runs; i++ {
		r, err := runOnce(client, *base, decodePrompt, 256, 0.75)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		tim, _ := r["timings"].(map[string]any)
		if tim != nil {
			if ps, ok := getFloat(tim, "predicted_per_second"); ok {
				decodeRates = append(decodeRates, ps)
			}
			if pn, ok := getInt(tim, "predicted_n"); ok {
				predictedNs = append(predictedNs, pn)
			}
		}
		if w, ok := r["wall_s"].(float64); ok {
			walls = append(walls, w)
		}
	}

	for i := 0; i < max(0, *prefillWarmup); i++ {
		_, _ = runOnce(client, *base, prefillPrompt, 4, 0.3)
	}
	for i := 0; i < *runs; i++ {
		r2, err := runOnce(client, *base, prefillPrompt, 8, 0.3)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		tim, _ := r2["timings"].(map[string]any)
		if tim != nil {
			if pr, ok := getFloat(tim, "prompt_per_second"); ok {
				promptRates = append(promptRates, pr)
			}
		}
	}

	decodeMap := map[string]any{
		"scenario":              "max_tokens=256, long user prompt",
		"decode_tokens_per_s":   summarize(decodeRates),
		"predicted_tokens_mean": nil,
		"wall_s_per_run_mean":   nil,
	}
	if len(predictedNs) > 0 {
		s := 0
		for _, n := range predictedNs {
			s += n
		}
		decodeMap["predicted_tokens_mean"] = float64(s) / float64(len(predictedNs))
	}
	if len(walls) > 0 {
		s := 0.0
		for _, w := range walls {
			s += w
		}
		decodeMap["wall_s_per_run_mean"] = s / float64(len(walls))
	}

	out := map[string]any{
		"llama_url": *base,
		"runs":      *runs,
		"decode":    decodeMap,
		"prefill": map[string]any{
			"scenario":            "~480-token article + short completion (max_tokens=8)",
			"warmup_repeats":      *prefillWarmup,
			"prompt_tokens_per_s": summarize(promptRates),
		},
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return
	}

	fmt.Println("llama-server:", *base)
	fmt.Println("Decode (server-reported predicted_per_second):", out["decode"].(map[string]any)["decode_tokens_per_s"])
	fmt.Println("Prefill (server-reported prompt_per_second):", out["prefill"].(map[string]any)["prompt_tokens_per_s"])
	if v, ok := decodeMap["predicted_tokens_mean"].(float64); ok {
		fmt.Printf("Mean completion tokens (actual): %.1f\n", v)
	}
}

func getenvDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
