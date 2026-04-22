// /usr/bin/env go run ./cmd/publish-ollama-hub-readme
//
// Publish Ollama Hub summary (≤255 chars) and readme for eslider/bonsai-1.7b.
// Built binary: ./bin/publish_ollama_hub_readme
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultSummary = "PrismML Bonsai 1.7B (GGUF Q1_0). Stock Ollama cannot load Q1_0 yet — run via " +
	"bonsai-ollama proxy + Prism llama-server: https://github.com/eSlider/bonsai-ollama"

func postForm(client *http.Client, target string, fields url.Values, cookie string, dryRun bool) (int, string, error) {
	body := fields.Encode()
	if dryRun {
		fmt.Printf("DRY-RUN POST %s\n  fields: %v\n  body bytes: %d\n", target, keysOf(fields), len(body))
		return 0, "", nil
	}
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "bonsai-ollama-publish (https://github.com/eSlider/bonsai-ollama)")
	res, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, "", err
	}
	return res.StatusCode, string(raw), nil
}

func keysOf(v url.Values) []string {
	ks := make([]string, 0, len(v))
	for k := range v {
		ks = append(ks, k)
	}
	return ks
}

func repoRoot() string {
	if r := strings.TrimSpace(os.Getenv("BONSAI_REPO_ROOT")); r != "" {
		return r
	}
	wd, err := os.Getwd()
	if err == nil {
		for d := wd; d != "/" && d != "."; d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
				return d
			}
		}
	}
	exe, err := os.Executable()
	if err == nil {
		d := filepath.Dir(exe)
		for ; d != "/" && d != "."; d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
				return d
			}
		}
	}
	return ""
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print actions only")
	flag.Parse()

	cookie := strings.TrimSpace(os.Getenv("OLLAMA_COM_COOKIE"))
	if cookie == "" && !*dryRun {
		fmt.Fprintln(os.Stderr, `Missing OLLAMA_COM_COOKIE.
  1. Sign in at https://ollama.com (account must own eslider/bonsai-1.7b).
  2. DevTools → Application → Cookies → https://ollama.com → copy Cookie header
     (or document.cookie in the console on ollama.com).
  3. export OLLAMA_COM_COOKIE='...'
  4. Re-run this program.`)
		os.Exit(1)
	}

	model := strings.Trim(strings.TrimSpace(os.Getenv("OLLAMA_HUB_MODEL")), "/")
	if model == "" {
		model = "eslider/bonsai-1.7b"
	}

	root := repoRoot()
	readmePath := strings.TrimSpace(os.Getenv("OLLAMA_HUB_README"))
	if readmePath == "" {
		readmePath = filepath.Join(root, "models", "bonsai-1.7b", "README.md")
	}
	summary := strings.TrimSpace(os.Getenv("OLLAMA_HUB_SUMMARY"))
	if summary == "" {
		summary = defaultSummary
	}
	if len([]rune(summary)) > 255 {
		fmt.Fprintf(os.Stderr, "Summary is %d runes; max 255 (Ollama hub limit).\n", len([]rune(summary)))
		os.Exit(1)
	}

	raw, err := os.ReadFile(readmePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Readme file not found: %s: %v\n", readmePath, err)
		os.Exit(1)
	}
	readmeText := strings.TrimSpace(string(raw))

	base := "https://ollama.com/" + model
	client := &http.Client{Timeout: 2 * time.Minute}

	steps := []struct {
		label  string
		fields url.Values
	}{
		{"summary", url.Values{"summary": {summary}}},
		{"readme", url.Values{"readme": {readmeText}}},
	}

	for _, step := range steps {
		code, body, err := postForm(client, base, step.fields, cookie, *dryRun)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if *dryRun {
			continue
		}
		if code != 200 {
			lim := body
			if len(lim) > 2000 {
				lim = lim[:2000]
			}
			fmt.Fprintf(os.Stderr, "POST %s failed: HTTP %d\n%s\n", step.label, code, lim)
			os.Exit(1)
		}
		v := step.fields.Get(step.label)
		fmt.Printf("OK: updated %s (%d chars)\n", step.label, len(v))
	}

	if *dryRun {
		fmt.Println("Dry run complete.")
	} else {
		fmt.Println("Done. View:", base)
	}
}
