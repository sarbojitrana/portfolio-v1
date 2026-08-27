// Command refresh rebuilds data/ from GitHub, Codeforces, LeetCode and Claude.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"portfolio/internal/capture"
	"portfolio/internal/codeforces"
	"portfolio/internal/describe"
	"portfolio/internal/ghapi"
	"portfolio/internal/leetcode"
	"portfolio/internal/model"
)

func main() {
	root := flag.String("root", ".", "repository root")
	user := flag.String("user", "sarbojitrana", "github handle")
	cfHandle := flag.String("codeforces", "Lord_Beerus", "codeforces handle")
	lcHandle := flag.String("leetcode", "sarbojit_007", "leetcode handle")
	only := flag.String("only", "all", "all|projects|coding|opensource|contributions")
	shots := flag.Bool("shots", true, "capture deployment previews")
	ai := flag.Bool("ai", true, "ask Claude to classify and describe repositories")
	flag.Parse()

	r := runner{root: *root, gh: ghapi.New(*user), user: *user}
	if err := r.loadOverrides(); err != nil {
		log.Fatal(err)
	}

	run := func(name string, fn func() error) {
		if *only != "all" && *only != name {
			return
		}
		log.Printf("refresh %s", name)
		if err := fn(); err != nil {
			log.Fatalf("%s: %v", name, err)
		}
	}

	run("contributions", r.contributions)
	run("coding", func() error { return r.coding(*cfHandle, *lcHandle) })
	run("opensource", r.opensource)
	run("projects", func() error { return r.projects(*shots, *ai) })
	log.Print("done")
}

type runner struct {
	root      string
	user      string
	gh        *ghapi.Client
	overrides model.Overrides
}

func (r *runner) path(name string) string { return filepath.Join(r.root, "data", name) }

func (r *runner) loadOverrides() error {
	b, err := os.ReadFile(r.path("overrides.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &r.overrides)
}

func (r *runner) write(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path(name), append(b, '\n'), 0o644)
}

func (r *runner) contributions() error {
	c, err := r.gh.Contributions()
	if err != nil {
		return err
	}
	return r.write("contributions.json", model.Contributions{
		Start: c.Start, Levels: c.Levels, Counts: c.Counts,
		Total: c.Total, ActiveDays: c.ActiveDays,
		LongestStreak: c.LongestStreak, BusiestDay: c.BusiestDay,
	})
}

func (r *runner) coding(cf, lc string) error {
	a, err := codeforces.Fetch(cf)
	if err != nil {
		return err
	}
	b, err := leetcode.Fetch(lc)
	if err != nil {
		return err
	}
	return r.write("coding.json", model.Coding{Codeforces: a, Leetcode: b})
}

func (r *runner) opensource() error {
	prs, total, err := r.gh.MergedPRs()
	if err != nil {
		return err
	}
	out := model.OpenSource{MergedTotal: total, PageSize: 5, OwnRepoPRs: total - len(prs)}
	repos := map[string]bool{}
	for _, p := range prs {
		repos[p.Repo] = true
		out.PRs = append(out.PRs, model.PR{
			Repo:   p.Repo,
			Number: p.Number,
			Title:  cleanTitle(p.Title),
			Diff:   fmt.Sprintf("+%d &minus;%d", p.Additions, p.Deletions),
			Merged: p.MergedAt.Format("2006-01-02"),
		})
	}
	out.UpstreamRepos = len(repos)

	var existing model.OpenSource
	if b, err := os.ReadFile(r.path("opensource.json")); err == nil {
		_ = json.Unmarshal(b, &existing)
		out.PageSize = existing.PageSize
		titles := map[string]string{}
		for _, p := range existing.PRs {
			titles[fmt.Sprintf("%s#%d", p.Repo, p.Number)] = p.Title
		}
		for i, p := range out.PRs {
			if t, ok := titles[fmt.Sprintf("%s#%d", p.Repo, p.Number)]; ok {
				out.PRs[i].Title = t
			}
		}
	}
	return r.write("opensource.json", out)
}

func (r *runner) projects(shots, useAI bool) error {
	repos, err := r.gh.Repos()
	if err != nil {
		return err
	}
	var cl describe.Client
	if useAI {
		cl = describe.New()
	}
	ctx := context.Background()

	out := model.Projects{Scanned: len(repos), Refreshed: time.Now().UTC().Format("2006-01-02")}
	var previous model.Projects
	if b, err := os.ReadFile(r.path("projects.json")); err == nil {
		_ = json.Unmarshal(b, &previous)
	}
	known := map[string]model.Project{}
	for _, p := range previous.Projects {
		known[p.Repo] = p
	}

	for _, repo := range repos {
		if repo.Fork || slices.Contains(r.overrides.Never, repo.Name) {
			continue
		}
		if _, merged := r.overrides.Merge[repo.Name]; merged {
			continue
		}
		pinned := slices.Contains(r.overrides.Always, repo.Name)

		p, ok := known[repo.Name]
		fresh := !ok || repo.PushedAt.After(parseDay(previous.Refreshed))
		if useAI && fresh {
			v, err := cl.Classify(ctx, describe.Repo{
				Name: repo.Name, Description: repo.Description, Language: repo.Language,
				Topics: repo.Topics, Homepage: repo.Homepage, Stars: repo.Stars,
				SizeKB: repo.Size, Readme: r.gh.Readme(repo.Name),
			})
			if err != nil {
				log.Printf("  %s: %v", repo.Name, err)
				if !ok {
					continue
				}
			} else {
				if !v.Publish && !pinned {
					log.Printf("  %s: held back (%s)", repo.Name, v.Reason)
					continue
				}
				p = model.Project{Repo: repo.Name, Name: v.Name, Kind: v.Kind, Blurb: v.Blurb, Facts: v.Facts}
			}
		}
		if p.Repo == "" {
			if !pinned {
				continue
			}
			p = model.Project{Repo: repo.Name, Name: repo.Name, Kind: "TOOL"}
		}

		p.Wide = slices.Contains(r.overrides.Wide, repo.Name)
		if len(p.Links) == 0 {
			if repo.Homepage != "" {
				p.Links = append(p.Links, model.Link{Label: "Live ↗", URL: repo.Homepage})
			}
			p.Links = append(p.Links, model.Link{Label: "Source ↗", URL: repo.HTMLURL})
		}
		applyOverride(&p, r.overrides.Entries[repo.Name])

		if shots && repo.Homepage != "" && p.Terminal == "" {
			file := repo.Name + ".jpg"
			dst := filepath.Join(r.root, "assets", "previews", file)
			if err := capture.Shot(repo.Homepage, dst); err != nil {
				log.Printf("  %s: no preview (%v)", repo.Name, err)
			} else {
				p.Preview = &model.Preview{
					File:  file,
					Alt:   p.Name + " — live deployment",
					Stamp: "CAPTURED " + out.Refreshed,
				}
			}
		}
		out.Projects = append(out.Projects, p)
	}

	order := r.overrides.Order
	slices.SortStableFunc(out.Projects, func(a, b model.Project) int {
		return rank(order, a.Repo) - rank(order, b.Repo)
	})
	out.Published = len(out.Projects)
	return r.write("projects.json", out)
}

func applyOverride(p *model.Project, o model.OverrideEntry) {
	if o.Name != "" {
		p.Name = o.Name
	}
	if o.Kind != "" {
		p.Kind = o.Kind
	}
	if o.Blurb != "" {
		p.Blurb = o.Blurb
	}
	if len(o.Facts) > 0 {
		p.Facts = o.Facts
	}
	if len(o.Links) > 0 {
		p.Links = o.Links
	}
	if o.Terminal != "" {
		p.Terminal = o.Terminal
		p.Preview = nil
	}
	if o.Preview != nil {
		p.Preview = o.Preview
	}
}

func rank(order []string, repo string) int {
	if i := slices.Index(order, repo); i >= 0 {
		return i
	}
	return len(order)
}

func parseDay(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func cleanTitle(t string) string {
	if i := strings.Index(t, ": "); i > 0 && i < 24 && !strings.Contains(t[:i], " ") {
		t = t[i+2:]
	}
	if r := []rune(t); len(r) > 0 {
		t = strings.ToUpper(string(r[0])) + string(r[1:])
	}
	return t
}
