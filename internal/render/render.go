// Package render turns data/ plus web/ into the finished page.
package render

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"portfolio/internal/model"
)

type Page struct {
	Profile       model.Profile
	Projects      model.Projects
	OpenSource    model.OpenSource
	Coding        model.Coding
	Contributions model.Contributions
	HeldBack      int
	Pages         int
	StyleTag      template.HTML
	ScriptTag     template.HTML
	DataJSON      template.JS
}

type Site struct {
	Root   string
	Inline bool
}

func (s Site) load(name string, into any) error {
	b, err := os.ReadFile(filepath.Join(s.Root, "data", name))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, into)
}

func (s Site) assetPath(file string) (string, error) {
	for _, p := range []string{
		filepath.Join(s.Root, "assets", "previews", file),
		filepath.Join(s.Root, "assets", file),
	} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("asset not found: %s", file)
}

func (s Site) asset(file string) (string, error) {
	p, err := s.assetPath(file)
	if err != nil {
		return "", err
	}
	if !s.Inline {
		rel, err := filepath.Rel(s.Root, p)
		if err != nil {
			return "", err
		}
		return filepath.ToSlash(rel), nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	mime := "image/jpeg"
	if strings.HasSuffix(strings.ToLower(p), ".png") {
		mime = "image/png"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

func (s Site) Render() ([]byte, error) {
	var page Page
	if err := s.load("profile.json", &page.Profile); err != nil {
		return nil, err
	}
	if err := s.load("projects.json", &page.Projects); err != nil {
		return nil, err
	}
	if err := s.load("opensource.json", &page.OpenSource); err != nil {
		return nil, err
	}
	if err := s.load("coding.json", &page.Coding); err != nil {
		return nil, err
	}
	if err := s.load("contributions.json", &page.Contributions); err != nil {
		return nil, err
	}
	page.Projects.Published = len(page.Projects.Projects)
	page.HeldBack = page.Projects.Scanned - countRepos(page.Projects)
	if page.OpenSource.PageSize > 0 {
		page.Pages = (len(page.OpenSource.PRs) + page.OpenSource.PageSize - 1) / page.OpenSource.PageSize
	}

	data, err := s.pageData(page)
	if err != nil {
		return nil, err
	}
	page.DataJSON = template.JS(data)

	css, err := os.ReadFile(filepath.Join(s.Root, "web", "style.css"))
	if err != nil {
		return nil, err
	}
	js, err := os.ReadFile(filepath.Join(s.Root, "web", "app.js"))
	if err != nil {
		return nil, err
	}
	if s.Inline {
		page.StyleTag = template.HTML("<style>\n" + string(css) + "</style>")
		page.ScriptTag = template.HTML("<script>\n" + string(js) + "</script>")
	} else {
		page.StyleTag = `<link rel="stylesheet" href="style.css">`
		page.ScriptTag = `<script src="app.js"></script>`
	}

	var assetErr error
	funcs := template.FuncMap{
		"safe":      func(v string) template.HTML { return template.HTML(v) },
		"inc":       func(i int) int { return i + 1 },
		"pad":       func(i int) string { return fmt.Sprintf("%02d", i) },
		"lower":     strings.ToLower,
		"comma":     comma,
		"hasPrefix": strings.HasPrefix,
		"safeURL":   func(v string) template.URL { return template.URL(v) },
		"asset": func(file string) template.URL {
			v, err := s.asset(file)
			if err != nil && assetErr == nil {
				assetErr = err
			}
			return template.URL(v)
		},
	}

	tmplPath := filepath.Join(s.Root, "web", "page.html.tmpl")
	t, err := template.New(filepath.Base(tmplPath)).Funcs(funcs).ParseFiles(tmplPath)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, page); err != nil {
		return nil, err
	}
	if assetErr != nil {
		return nil, assetErr
	}
	return buf.Bytes(), nil
}

// Document wraps the fragment in a full HTML document for the hosted site.
func Document(fragment []byte, desc string) []byte {
	body := string(fragment)
	head := ""
	if i := strings.Index(body, `<div class="sheet">`); i > 0 {
		head, body = body[:i], body[i:]
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n")
	b.WriteString("<meta name=\"description\" content=\"" + template.HTMLEscapeString(desc) + "\">\n")
	b.WriteString(strings.TrimSpace(head))
	b.WriteString("\n</head>\n<body>\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n</body>\n</html>\n")
	return []byte(b.String())
}

func (s Site) pageData(p Page) ([]byte, error) {
	type platform struct {
		Ratings   []int             `json:"ratings"`
		Bands     []json.RawMessage `json:"bands"`
		Histogram []json.RawMessage `json:"histogram"`
	}
	payload := struct {
		Codeforces    platform            `json:"codeforces"`
		Leetcode      platform            `json:"leetcode"`
		Languages     []json.RawMessage   `json:"languages"`
		Contributions model.Contributions `json:"contributions"`
		PRs           []model.PR          `json:"prs"`
		PRPageSize    int                 `json:"prPageSize"`
		RequestPath   struct {
			Capacity   int `json:"capacity"`
			RefillMs   int `json:"refillMs"`
			CacheTTLMs int `json:"cacheTtlMs"`
		} `json:"requestPath"`
	}{
		Codeforces:    platform{p.Coding.Codeforces.Ratings, p.Coding.Codeforces.Bands, p.Coding.Codeforces.Histogram},
		Leetcode:      platform{p.Coding.Leetcode.Ratings, p.Coding.Leetcode.Bands, p.Coding.Leetcode.Histogram},
		Languages:     p.Profile.Github.Languages,
		Contributions: p.Contributions,
		PRs:           p.OpenSource.PRs,
		PRPageSize:    p.OpenSource.PageSize,
	}
	payload.RequestPath.Capacity = p.Profile.RequestPath.Capacity
	payload.RequestPath.RefillMs = p.Profile.RequestPath.RefillMs
	payload.RequestPath.CacheTTLMs = p.Profile.RequestPath.CacheTTLMs
	return json.Marshal(payload)
}

func countRepos(p model.Projects) int {
	n := 0
	for _, pr := range p.Projects {
		n++
		for _, l := range pr.Links {
			if l.Label == "Client ↗" {
				n++
			}
		}
	}
	return n
}

func comma(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
