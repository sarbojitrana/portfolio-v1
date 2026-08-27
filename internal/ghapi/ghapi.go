// Package ghapi reads the public GitHub surface: repos, merged PRs, contributions.
package ghapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	User  string
	Token string
	HTTP  *http.Client
}

func New(user string) *Client {
	return &Client{User: user, Token: os.Getenv("GITHUB_TOKEN"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

type User struct {
	Login       string    `json:"login"`
	PublicRepos int       `json:"public_repos"`
	Followers   int       `json:"followers"`
	CreatedAt   time.Time `json:"created_at"`
	AvatarURL   string    `json:"avatar_url"`
}

type Repo struct {
	Name        string    `json:"name"`
	Fork        bool      `json:"fork"`
	Archived    bool      `json:"archived"`
	Language    string    `json:"language"`
	Homepage    string    `json:"homepage"`
	Description string    `json:"description"`
	Topics      []string  `json:"topics"`
	Size        int       `json:"size"`
	Stars       int       `json:"stargazers_count"`
	PushedAt    time.Time `json:"pushed_at"`
	HTMLURL     string    `json:"html_url"`
	Readme      string    `json:"-"`
}

type PullRequest struct {
	Repo      string
	Number    int
	Title     string
	State     string
	At        time.Time
	Additions int
	Deletions int
}

func (c *Client) get(path string, into any) error {
	req, err := http.NewRequest("GET", "https://api.github.com"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 400))
		return fmt.Errorf("GET %s: %s: %s", path, res.Status, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(res.Body).Decode(into)
}

func (c *Client) User_() (User, error) {
	var u User
	err := c.get("/users/"+c.User, &u)
	return u, err
}

func (c *Client) Repos() ([]Repo, error) {
	var all []Repo
	for page := 1; ; page++ {
		var batch []Repo
		if err := c.get(fmt.Sprintf("/users/%s/repos?per_page=100&page=%d", c.User, page), &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].PushedAt.After(all[j].PushedAt) })
	return all, nil
}

func (c *Client) Readme(repo string) string {
	res, err := c.HTTP.Get("https://raw.githubusercontent.com/" + c.User + "/" + repo + "/HEAD/README.md")
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 20_000))
	if err != nil {
		return ""
	}
	return string(b)
}

// PullRequests returns every pull request the user opened in a repository they
// do not own, tagged merged, open or closed.
func (c *Client) PullRequests() ([]PullRequest, int, error) {
	q := url.QueryEscape(fmt.Sprintf("author:%s is:pr", c.User))
	var page struct {
		Total int `json:"total_count"`
		Items []struct {
			Number      int        `json:"number"`
			Title       string     `json:"title"`
			State       string     `json:"state"`
			RepoURL     string     `json:"repository_url"`
			CreatedAt   time.Time  `json:"created_at"`
			ClosedAt    *time.Time `json:"closed_at"`
			PullRequest struct {
				MergedAt *time.Time `json:"merged_at"`
			} `json:"pull_request"`
		} `json:"items"`
	}
	if err := c.get("/search/issues?per_page=100&sort=created&order=desc&q="+q, &page); err != nil {
		return nil, 0, err
	}
	var out []PullRequest
	for _, it := range page.Items {
		repo := strings.TrimPrefix(it.RepoURL, "https://api.github.com/repos/")
		if strings.HasPrefix(repo, c.User+"/") {
			continue
		}
		pr := PullRequest{Repo: repo, Number: it.Number, Title: it.Title, State: "open", At: it.CreatedAt}
		switch {
		case it.PullRequest.MergedAt != nil:
			pr.State, pr.At = "merged", *it.PullRequest.MergedAt
		case it.State == "closed":
			pr.State = "closed"
			if it.ClosedAt != nil {
				pr.At = *it.ClosedAt
			}
		}
		var detail struct {
			Additions int `json:"additions"`
			Deletions int `json:"deletions"`
		}
		if err := c.get(fmt.Sprintf("/repos/%s/pulls/%d", repo, it.Number), &detail); err == nil {
			pr.Additions, pr.Deletions = detail.Additions, detail.Deletions
		}
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, page.Total, nil
}

type Contributions struct {
	Start         string
	Levels        string
	Counts        string
	Total         int
	ActiveDays    int
	LongestStreak int
	BusiestDay    int
}

var (
	cellRe = regexp.MustCompile(`<td[^>]*data-date="(\d{4}-\d{2}-\d{2})"[^>]*id="(contribution-day-component-\d+-\d+)"[^>]*data-level="(\d)"`)
	tipRe  = regexp.MustCompile(`<tool-tip[^>]*for="(contribution-day-component-\d+-\d+)"[^>]*>([^<]*)</tool-tip>`)
	numRe  = regexp.MustCompile(`^([\d,]+) contribution`)
)

const base36 = "0123456789abcdefghijklmnopqrstuvwxyz"

func (c *Client) Contributions() (Contributions, error) {
	req, _ := http.NewRequest("GET", "https://github.com/users/"+c.User+"/contributions", nil)
	req.Header.Set("User-Agent", "portfolio-refresh")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Contributions{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return Contributions{}, err
	}
	html := string(body)

	tips := map[string]string{}
	for _, m := range tipRe.FindAllStringSubmatch(html, -1) {
		tips[m[1]] = m[2]
	}
	type day struct {
		date  string
		level string
		count int
	}
	var days []day
	for _, m := range cellRe.FindAllStringSubmatch(html, -1) {
		n := 0
		if g := numRe.FindStringSubmatch(tips[m[2]]); g != nil {
			n, _ = strconv.Atoi(strings.ReplaceAll(g[1], ",", ""))
		}
		days = append(days, day{m[1], m[3], n})
	}
	if len(days) == 0 {
		return Contributions{}, fmt.Errorf("no contribution cells found")
	}
	sort.Slice(days, func(i, j int) bool { return days[i].date < days[j].date })

	out := Contributions{Start: days[0].date}
	var levels, counts strings.Builder
	streak := 0
	for _, d := range days {
		levels.WriteString(d.level)
		counts.WriteByte(base36[min(d.count, 35)])
		out.Total += d.count
		out.BusiestDay = max(out.BusiestDay, d.count)
		if d.count > 0 {
			out.ActiveDays++
			streak++
			out.LongestStreak = max(out.LongestStreak, streak)
		} else {
			streak = 0
		}
	}
	out.Levels, out.Counts = levels.String(), counts.String()
	return out, nil
}
