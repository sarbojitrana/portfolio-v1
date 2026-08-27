// Package describe asks Claude which repositories are projects and writes their blurbs.
package describe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

const Model = "claude-opus-5"

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
chips (at most 6 words each) naming the stack and the load-bearing numbers.

Reply with one JSON object and nothing else:
{"publish":bool,"reason":"short phrase","kind":"SERVICE|CLI|TOOL","name":"display name",
 "blurb":"...","facts":["GO","POSTGRES"]}

kind: SERVICE for anything that serves requests, CLI for terminal programs, TOOL for
everything else that runs locally.`

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

type Client struct{ api anthropic.Client }

func New() Client { return Client{api: anthropic.NewClient()} }

func (c Client) Classify(ctx context.Context, r Repo) (Verdict, error) {
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

	adaptive := anthropic.ThinkingConfigAdaptiveParam{}
	res, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     Model,
		MaxTokens: 4000,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive},
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(b.String()))},
	})
	if err != nil {
		return Verdict{}, err
	}
	if res.StopReason == anthropic.StopReasonRefusal {
		return Verdict{}, fmt.Errorf("refused: %s", res.StopDetails.Category)
	}

	var text strings.Builder
	for _, block := range res.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
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
