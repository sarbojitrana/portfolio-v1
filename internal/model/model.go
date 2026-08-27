// Package model holds the shapes of everything under data/.
package model

import "encoding/json"

type Profile struct {
	Name         string        `json:"name"`
	First        string        `json:"first"`
	Last         string        `json:"last"`
	Role         string        `json:"role"`
	Yearbox      Yearbox       `json:"yearbox"`
	Status       string        `json:"status"`
	Pill         string        `json:"pill"`
	Overview     Overview      `json:"overview"`
	RequestPath  RequestPath   `json:"request_path"`
	Achievements []Achievement `json:"achievements"`
	Github       Github        `json:"github"`
	Contact      []Contact     `json:"contact"`
	Resume       Resume        `json:"resume"`
	Pipeline     Pipeline      `json:"pipeline"`
	PracticeNote string        `json:"practice_note"`
	Footer       []string      `json:"footer"`
}

type Yearbox struct {
	Left  string `json:"left"`
	Right string `json:"right"`
}

type Overview struct {
	Eyebrow    string    `json:"eyebrow"`
	Paragraphs []string  `json:"paragraphs"`
	Spec       []SpecRow `json:"spec"`
}

type SpecRow struct {
	K string `json:"k"`
	V string `json:"v"`
}

type RequestPath struct {
	Intro      string  `json:"intro"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Capacity   int     `json:"capacity"`
	RefillMs   int     `json:"refill_ms"`
	CacheTTLMs int     `json:"cache_ttl_ms"`
	Stages     []Stage `json:"stages"`
}

type Stage struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Sub  string `json:"sub"`
	Note string `json:"note"`
}

type Achievement struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type Github struct {
	Handle               string            `json:"handle"`
	URL                  string            `json:"url"`
	MemberSince          string            `json:"member_since"`
	Stats                []Stat            `json:"stats"`
	Languages            []json.RawMessage `json:"languages"`
	AuthoredWithLanguage int               `json:"authored_with_language"`
	Prose                []string          `json:"prose"`
}

type Stat struct {
	N string `json:"n"`
	L string `json:"l"`
}

type Contact struct {
	K   string `json:"k"`
	V   string `json:"v"`
	URL string `json:"url"`
}

type Resume struct {
	File string `json:"file"`
	Meta string `json:"meta"`
	Raw  string `json:"raw"`
	Blob string `json:"blob"`
	Note string `json:"note"`
}

type Pipeline struct {
	Intro   string     `json:"intro"`
	Steps   []Step     `json:"steps"`
	Include []string   `json:"include"`
	Exclude []Excluded `json:"exclude"`
}

type Step struct {
	N     string `json:"n"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type Excluded struct {
	Count string `json:"count"`
	Text  string `json:"text"`
}

type Projects struct {
	Scanned   int       `json:"scanned"`
	Published int       `json:"published"`
	Refreshed string    `json:"refreshed"`
	Projects  []Project `json:"projects"`
}

type Project struct {
	Repo     string   `json:"repo"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Wide     bool     `json:"wide"`
	Blurb    string   `json:"blurb"`
	Facts    []string `json:"facts"`
	Links    []Link   `json:"links"`
	Preview  *Preview `json:"preview,omitempty"`
	Terminal string   `json:"terminal,omitempty"`
}

type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type Preview struct {
	File  string `json:"file"`
	Alt   string `json:"alt"`
	Stamp string `json:"stamp"`
}

type OpenSource struct {
	MergedTotal   int  `json:"merged_total"`
	OpenedTotal   int  `json:"opened_total"`
	UpstreamRepos int  `json:"upstream_repos"`
	OwnRepoPRs    int  `json:"own_repo_prs"`
	PageSize      int  `json:"page_size"`
	PRs           []PR `json:"prs"`
}

type PR struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Diff   string `json:"diff"`
	Merged string `json:"merged"`
}

type Coding struct {
	Codeforces Platform `json:"codeforces"`
	Leetcode   Platform `json:"leetcode"`
}

type Platform struct {
	Handle      string            `json:"handle"`
	URL         string            `json:"url"`
	Peak        int               `json:"peak"`
	PeakRank    string            `json:"peak_rank,omitempty"`
	PeakContest string            `json:"peak_contest,omitempty"`
	PeakDate    string            `json:"peak_date,omitempty"`
	Current     int               `json:"current"`
	CurrentRank string            `json:"current_rank,omitempty"`
	Badge       string            `json:"badge,omitempty"`
	TopPercent  float64           `json:"top_percent,omitempty"`
	GlobalRank  int               `json:"global_rank,omitempty"`
	Contests    int               `json:"contests"`
	Solved      int               `json:"solved"`
	Submissions int               `json:"submissions,omitempty"`
	Acceptance  float64           `json:"acceptance"`
	First       string            `json:"first"`
	Last        string            `json:"last"`
	Ratings     []int             `json:"ratings"`
	Bands       []json.RawMessage `json:"bands"`
	Histogram   []json.RawMessage `json:"histogram"`
}

type Contributions struct {
	Start         string `json:"start"`
	Levels        string `json:"levels"`
	Counts        string `json:"counts"`
	Total         int    `json:"total"`
	ActiveDays    int    `json:"active_days"`
	LongestStreak int    `json:"longest_streak"`
	BusiestDay    int    `json:"busiest_day"`
}

// Overrides pins what the classifier cannot decide on its own.
type Overrides struct {
	Always  []string                 `json:"always"`
	Never   []string                 `json:"never"`
	Wide    []string                 `json:"wide"`
	Order   []string                 `json:"order"`
	Merge   map[string]string        `json:"merge"`
	Entries map[string]OverrideEntry `json:"entries"`
}

type OverrideEntry struct {
	Name     string   `json:"name,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Blurb    string   `json:"blurb,omitempty"`
	Facts    []string `json:"facts,omitempty"`
	Terminal string   `json:"terminal,omitempty"`
	Links    []Link   `json:"links,omitempty"`
	Preview  *Preview `json:"preview,omitempty"`
}
