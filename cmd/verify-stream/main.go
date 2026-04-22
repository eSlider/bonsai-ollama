// /usr/bin/env go run ./cmd/verify-stream
//
// Read Ollama NDJSON from stdin; check streaming is active and chunks arrive without long stalls.
// Built binary: ./bin/verify_stream
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

func main() {
	prev := time.Now()
	var maxGapMs float64
	nonDone := 0
	maxCP := 0
	type chunk struct {
		n   int
		pre string
	}
	var chunks []chunk

	sc := bufio.NewScanner(os.Stdin)
	// Default token limit may be too small for huge lines; cap buffer.
	const maxTok = 16 * 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, maxTok)

	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		now := time.Now()
		gapMs := now.Sub(prev).Seconds() * 1000.0
		if gapMs > maxGapMs {
			maxGapMs = gapMs
		}
		prev = now

		var o map[string]any
		if err := json.Unmarshal([]byte(raw), &o); err != nil {
			pre := raw
			if len(pre) > 120 {
				pre = pre[:120]
			}
			fmt.Fprintf(os.Stderr, "BAD_JSON %v %s\n", err, pre)
			os.Exit(2)
		}
		if done, _ := o["done"].(bool); done {
			continue
		}
		var c string
		if msg, ok := o["message"].(map[string]any); ok {
			if s, ok := msg["content"].(string); ok {
				c = s
			}
		}
		if c == "" {
			if s, ok := o["response"].(string); ok {
				c = s
			}
		}
		ncp := utf8.RuneCountInString(c)
		if ncp > maxCP {
			maxCP = ncp
		}
		nonDone++
		pre := c
		runes := []rune(c)
		if len(runes) > 40 {
			pre = string(runes[:40])
		}
		chunks = append(chunks, chunk{n: ncp, pre: pre})
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	fmt.Println("non_done_chunks", nonDone)
	fmt.Println("max_codepoints_per_chunk", maxCP)
	fmt.Printf("max_inter_chunk_gap_ms %.3f\n", maxGapMs)
	if len(chunks) > 0 {
		n := 5
		if len(chunks) < n {
			n = len(chunks)
		}
		fmt.Print("first_chunks_preview [")
		for i := 0; i < n; i++ {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Printf("(%d, %q)", chunks[i].n, chunks[i].pre)
		}
		fmt.Println("]")
	}

	if nonDone == 0 {
		fmt.Fprintln(os.Stderr, "FAIL: no streaming chunks received")
		os.Exit(1)
	}
	if maxGapMs > 8000 {
		fmt.Fprintf(os.Stderr, "FAIL: long stall between chunks (ms) %v\n", maxGapMs)
		os.Exit(1)
	}
	fmt.Println("OK")
}
