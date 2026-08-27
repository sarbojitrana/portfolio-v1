// Package codeforces reads a public Codeforces profile.
package codeforces

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"portfolio/internal/model"
)

var client = &http.Client{Timeout: 60 * time.Second}

func get(url string, into any) error {
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("GET %s: %s", url, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(into)
}

func Fetch(handle string) (model.Platform, error) {
	var p model.Platform

	var info struct {
		Status string `json:"status"`
		Result []struct {
			Handle    string `json:"handle"`
			Rating    int    `json:"rating"`
			Rank      string `json:"rank"`
			MaxRating int    `json:"maxRating"`
			MaxRank   string `json:"maxRank"`
		} `json:"result"`
	}
	if err := get("https://codeforces.com/api/user.info?handles="+handle, &info); err != nil {
		return p, err
	}
	if len(info.Result) == 0 {
		return p, fmt.Errorf("codeforces: no such user %q", handle)
	}
	u := info.Result[0]

	var rating struct {
		Result []struct {
			NewRating  int   `json:"newRating"`
			UpdateTime int64 `json:"ratingUpdateTimeSeconds"`
		} `json:"result"`
	}
	if err := get("https://codeforces.com/api/user.rating?handle="+handle, &rating); err != nil {
		return p, err
	}

	var status struct {
		Result []struct {
			Verdict string `json:"verdict"`
			Problem struct {
				ContestID int    `json:"contestId"`
				Index     string `json:"index"`
				Rating    int    `json:"rating"`
			} `json:"problem"`
		} `json:"result"`
	}
	if err := get("https://codeforces.com/api/user.status?handle="+handle+"&from=1&count=100000", &status); err != nil {
		return p, err
	}

	solved := map[string]bool{}
	byRating := map[int]int{}
	accepted := 0
	for _, s := range status.Result {
		if s.Verdict != "OK" {
			continue
		}
		accepted++
		key := fmt.Sprintf("%d%s", s.Problem.ContestID, s.Problem.Index)
		if !solved[key] {
			solved[key] = true
			if s.Problem.Rating > 0 {
				byRating[s.Problem.Rating]++
			}
		}
	}
	bucket := func(lo, hi int) int {
		n := 0
		for r, c := range byRating {
			if r >= lo && r <= hi {
				n += c
			}
		}
		return n
	}

	p = model.Platform{
		Handle:      u.Handle,
		URL:         "https://codeforces.com/profile/" + u.Handle,
		Peak:        u.MaxRating,
		PeakRank:    u.MaxRank,
		Current:     u.Rating,
		CurrentRank: u.Rank,
		Contests:    len(rating.Result),
		Solved:      len(solved),
		Submissions: len(status.Result),
		Acceptance:  round1(100 * float64(accepted) / float64(max(1, len(status.Result)))),
	}
	for _, r := range rating.Result {
		p.Ratings = append(p.Ratings, r.NewRating)
	}
	if n := len(rating.Result); n > 0 {
		p.First = time.Unix(rating.Result[0].UpdateTime, 0).UTC().Format("Jan 2006")
		p.Last = time.Unix(rating.Result[n-1].UpdateTime, 0).UTC().Format("Jan 2006")
	}
	p.Bands = raw([][2]any{{1200, "pupil"}, {1400, "specialist"}})
	p.Histogram = raw([][2]any{
		{"800—1099", bucket(800, 1099)},
		{"1100—1299", bucket(1100, 1299)},
		{"1300—1499", bucket(1300, 1499)},
		{"1500—1699", bucket(1500, 1699)},
		{"1700—2200", bucket(1700, 2200)},
	})
	return p, nil
}

func raw(pairs [][2]any) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(pairs))
	for _, p := range pairs {
		b, _ := json.Marshal([]any{p[0], p[1]})
		out = append(out, b)
	}
	return out
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
