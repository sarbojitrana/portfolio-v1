// Package describe asks Gemini which repositories are projects and writes their blurbs.
package describe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultModel = "gemini-3.7-flash"
	endpoint     = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"
)

const system = `You curate the projects shelf of a backend engineer's portfolio. For one
repository you decide whether it belongs there, and if it does you write the entry.

Publish a repository only when all of these hold:
- it is source code the owner wrote, not a fork, mirror or template
- its README explains what the thing does, beyond a bare title
- it solves a problem for someone other than a grader or the author's coursework
- it is not notes, slides, coursework, a competitive-programming solution archive,
  a tutorial follow-along, an empty scaffold or a profile config repository

Write the blurb in the owner's voice: plain, concrete, present tense, 2-4 sentences.
Say what it does and the one design decision worth knowing. Never invent a fact the
README does not support; never use marketing adjectives. Facts are short uppercase
chips of at most 6 words each, naming the stack and the load-bearing numbers.

kind is SERVICE for anything that serves requests, CLI for terminal programs, TOOL for
everything else that runs locally. reason is a short phrase, and is what gets logged
when you hold a repository back.`

type Repo struct {
	Name        string
	Description string
	Language    string
	Topics      []string
	Homepage    string
	Stars       int
	SizeKB      int
	Readme      string
}

type Verdict struct {
	Publish bool     `json:"publish"`
	Reason  string   `json:"reason"`
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Blurb   string   `json:"blurb"`
	Facts   []string `json:"facts"`
}

type Client struct {
	Model  string
	APIKey string
	HTTP   *http.Client
}

func New() Client {
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = defaultModel
	}
	return Client{
		Model:  model,
		APIKey: os.Getenv("GEMINI_API_KEY"),
		HTTP:   &http.Client{Timeout: 90 * time.Second},
	}
}

var schema = map[string]any{
	"type": "OBJECT",
	"properties": map[string]any{
		"publish": map[string]any{"type": "BOOLEAN"},
		"reason":  map[string]any{"type": "STRING"},
		"kind":    map[string]any{"type": "STRING", "enum": []string{"SERVICE", "CLI", "TOOL"}},
		"name":    map[string]any{"type": "STRING"},
		"blurb":   map[string]any{"type": "STRING"},
		"facts":   map[string]any{"type": "ARRAY", "items": map[string]any{"type": "STRING"}},
	},
	"required": []string{"publish", "reason", "kind", "name", "blurb", "facts"},
}

type response struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func (c Client) Classify(ctx context.Context, r Repo) (Verdict, error) {
	if c.APIKey == "" {
		return Verdict{}, fmt.Errorf("GEMINI_API_KEY is not set")
	}
	body, err := json.Marshal(map[string]any{
		"systemInstruction": map[string]any{"parts": []any{map[string]string{"text": system}}},
		"contents": []any{map[string]any{
			"role":  "user",
			"parts": []any{map[string]string{"text": prompt(r)}},
		}},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
			"responseSchema":   schema,
			"temperature":      0.3,
		},
	})
	if err != nil {
		return Verdict{}, err
	}

	var res response
	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt*attempt*5) * time.Second):
			case <-ctx.Done():
				return Verdict{}, ctx.Err()
			}
		}
		res, lastErr = c.call(ctx, body)
		if lastErr == nil {
			break
		}
		if !retryable(lastErr) {
			return Verdict{}, lastErr
		}
	}
	if lastErr != nil {
		return Verdict{}, lastErr
	}

	if res.PromptFeedback.BlockReason != "" {
		return Verdict{}, fmt.Errorf("prompt blocked: %s", res.PromptFeedback.BlockReason)
	}
	if len(res.Candidates) == 0 {
		return Verdict{}, fmt.Errorf("no candidates returned")
	}
	if fr := res.Candidates[0].FinishReason; fr != "" && fr != "STOP" {
		return Verdict{}, fmt.Errorf("finished early: %s", fr)
	}
	var text strings.Builder
	for _, p := range res.Candidates[0].Content.Parts {
		text.WriteString(p.Text)
	}
	raw := firstObject(text.String())
	if raw == "" {
		return Verdict{}, fmt.Errorf("no JSON object in reply for %s", r.Name)
	}
	var v Verdict
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return Verdict{}, fmt.Errorf("decode reply for %s: %w", r.Name, err)
	}
	if v.Name == "" {
		v.Name = r.Name
	}
	return v, nil
}

type statusError struct {
	code int
	msg  string
}

func (e statusError) Error() string { return fmt.Sprintf("gemini: %d %s", e.code, e.msg) }

func retryable(err error) bool {
	var s statusError
	if errors.As(err, &s) {
		return s.code == 429 || s.code >= 500
	}
	return true
}

func (c Client) call(ctx context.Context, body []byte) (response, error) {
	url := fmt.Sprintf(endpoint, c.Model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.APIKey)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return response{}, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return response{}, err
	}
	var out response
	if err := json.Unmarshal(raw, &out); err != nil {
		return response{}, fmt.Errorf("gemini: %s: %s", res.Status, trim(string(raw)))
	}
	if res.StatusCode != http.StatusOK {
		msg := res.Status
		if out.Error != nil {
			msg = out.Error.Message
		}
		return response{}, statusError{code: res.StatusCode, msg: msg}
	}
	return out, nil
}

func prompt(r Repo) string {
	readme := r.Readme
	if len(readme) > 8000 {
		readme = readme[:8000]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "repository: %s\n", r.Name)
	fmt.Fprintf(&b, "description: %s\n", or(r.Description, "(none)"))
	fmt.Fprintf(&b, "language: %s\n", or(r.Language, "(none)"))
	fmt.Fprintf(&b, "topics: %s\n", or(strings.Join(r.Topics, ", "), "(none)"))
	fmt.Fprintf(&b, "homepage: %s\n", or(r.Homepage, "(none)"))
	fmt.Fprintf(&b, "stars: %d\nsize: %d kB\n\nREADME:\n%s\n", r.Stars, r.SizeKB, or(readme, "(no README)"))
	return b.String()
}

func firstObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth, inString, escaped := 0, false, false
	for i := start; i < len(s); i++ {
		switch {
		case escaped:
			escaped = false
		case s[i] == '\\' && inString:
			escaped = true
		case s[i] == '"':
			inString = !inString
		case inString:
		case s[i] == '{':
			depth++
		case s[i] == '}':
			if depth--; depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func or(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
