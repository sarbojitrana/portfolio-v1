// Command refresh rebuilds data/ from GitHub, Codeforces, LeetCode and Gemini.
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
	"strconv"
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
	ai := flag.Bool("ai", true, "ask Gemini to classify and describe repositories")
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
	prs, total, err := r.gh.PullRequests()
	if err != nil {
		return err
	}
	out := model.OpenSource{OpenedTotal: total, PageSize: 5, DefaultState: "merged", OwnRepoPRs: total - len(prs)}
	repos := map[string]bool{}
	for _, p := range prs {
		if slices.Contains(r.overrides.NeverPRs, p.Repo) {
			continue
		}
		repos[p.Repo] = true
		if p.State == "merged" {
			out.MergedTotal++
		}
		out.PRs = append(out.PRs, model.PR{
			Repo:   p.Repo,
			Number: p.Number,
			Title:  cleanTitle(p.Title),
			Diff:   fmt.Sprintf("+%d &minus;%d", p.Additions, p.Deletions),
			State:  p.State,
			Date:   p.At.Format("2006-01-02"),
		})
	}
	out.UpstreamRepos = len(repos)

	var existing model.OpenSource
	if b, err := os.ReadFile(r.path("opensource.json")); err == nil {
		_ = json.Unmarshal(b, &existing)
		if existing.PageSize > 0 {
			out.PageSize = existing.PageSize
		}
		if existing.DefaultState != "" {
			out.DefaultState = existing.DefaultState
		}
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

	var forks, byHand, notPublished int
	for _, repo := range repos {
		if repo.Fork {
			forks++
			continue
		}
		if slices.Contains(r.overrides.Never, repo.Name) {
			byHand++
			continue
		}
		if _, merged := r.overrides.Merge[repo.Name]; merged {
			byHand++
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
					notPublished++
					continue
				}
				p = model.Project{Repo: repo.Name, Name: v.Name, Kind: v.Kind, Blurb: v.Blurb, Facts: v.Facts}
			}
		}
		if p.Repo == "" {
			if !pinned {
				notPublished++
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
	if limit := r.overrides.Limit; limit > 0 && len(out.Projects) > limit {
		byHand += len(out.Projects) - limit
		out.Projects = out.Projects[:limit]
	}
	if err := r.githubBlock(repos); err != nil {
		return err
	}
	out.Published = len(out.Projects)
	out.Held = []model.Excluded{
		{Count: itoa(forks), Text: "Forks — routed to the open-source ledger instead"},
		{Count: itoa(notPublished), Text: "Notes, coursework, solution archives, tutorial builds and scaffolding"},
		{Count: itoa(byHand), Text: "Set aside by hand in overrides.json, or folded into another card"},
	}
	return r.write("projects.json", out)
}

// githubBlock refreshes the counts and language mix shown in section 06.
func (r *runner) githubBlock(repos []ghapi.Repo) error {
	b, err := os.ReadFile(r.path("profile.json"))
	if err != nil {
		return err
	}
	var p model.Profile
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	user, err := r.gh.User_()
	if err != nil {
		return err
	}
	prs, openedTotal, err := r.gh.PullRequests()
	if err != nil {
		return err
	}
	merged := 0
	for _, p := range prs {
		if p.State == "merged" && !slices.Contains(r.overrides.NeverPRs, p.Repo) {
			merged++
		}
	}
	_ = openedTotal

	authored, forks := 0, 0
	byLang := map[string]int{}
	for _, repo := range repos {
		if repo.Fork {
			forks++
			continue
		}
		authored++
		if repo.Language != "" {
			byLang[repo.Language]++
		}
	}
	langs := make([]string, 0, len(byLang))
	for l := range byLang {
		langs = append(langs, l)
	}
	slices.SortFunc(langs, func(a, b string) int {
		if byLang[a] != byLang[b] {
			return byLang[b] - byLang[a]
		}
		return strings.Compare(a, b)
	})

	withLanguage, other := 0, 0
	p.Github.Languages = p.Github.Languages[:0]
	for i, l := range langs {
		withLanguage += byLang[l]
		if i < 6 {
			raw, _ := json.Marshal([]any{l, byLang[l]})
			p.Github.Languages = append(p.Github.Languages, raw)
			continue
		}
		other += byLang[l]
	}
	if other > 0 {
		raw, _ := json.Marshal([]any{"Other", other})
		p.Github.Languages = append(p.Github.Languages, raw)
	}
	p.Github.AuthoredWithLanguage = withLanguage
	p.Github.MemberSince = "member since " + strings.ToLower(user.CreatedAt.Format("Jan 2006"))
	p.Github.Stats = []model.Stat{
		{N: itoa(user.PublicRepos), L: "public repos"},
		{N: itoa(authored), L: fmt.Sprintf("authored / %d forked", forks)},
		{N: itoa(merged), L: "merged pull requests"},
		{N: itoa(user.Followers), L: "followers"},
	}

	out, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path("profile.json"), append(out, '\n'), 0o644)
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

func itoa(n int) string { return strconv.Itoa(n) }

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
