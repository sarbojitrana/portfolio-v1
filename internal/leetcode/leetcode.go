// Package leetcode reads a public LeetCode profile through the GraphQL endpoint.
package leetcode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"portfolio/internal/model"
)

const endpoint = "https://leetcode.com/graphql"

const query = `query profile($u: String!) {
  matchedUser(username: $u) {
    submitStats: submitStatsGlobal {
      acSubmissionNum { difficulty count submissions }
      totalSubmissionNum { difficulty count submissions }
    }
  }
  userContestRanking(username: $u) {
    attendedContestsCount rating globalRanking topPercentage badge { name }
  }
  userContestRankingHistory(username: $u) {
    attended rating contest { title startTime }
  }
}`

type response struct {
	Data struct {
		MatchedUser *struct {
			SubmitStats struct {
				AcSubmissionNum    []bucket `json:"acSubmissionNum"`
				TotalSubmissionNum []bucket `json:"totalSubmissionNum"`
			} `json:"submitStats"`
		} `json:"matchedUser"`
		UserContestRanking *struct {
			AttendedContestsCount int     `json:"attendedContestsCount"`
			Rating                float64 `json:"rating"`
			GlobalRanking         int     `json:"globalRanking"`
			TopPercentage         float64 `json:"topPercentage"`
			Badge                 *struct {
				Name string `json:"name"`
			} `json:"badge"`
		} `json:"userContestRanking"`
		History []struct {
			Attended bool    `json:"attended"`
			Rating   float64 `json:"rating"`
			Contest  struct {
				Title     string `json:"title"`
				StartTime int64  `json:"startTime"`
			} `json:"contest"`
		} `json:"userContestRankingHistory"`
	} `json:"data"`
}

type bucket struct {
	Difficulty  string `json:"difficulty"`
	Count       int    `json:"count"`
	Submissions int    `json:"submissions"`
}

func Fetch(handle string) (model.Platform, error) {
	var p model.Platform
	body, _ := json.Marshal(map[string]any{"query": query, "variables": map[string]string{"u": handle}})
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return p, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "portfolio-refresh")
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return p, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return p, fmt.Errorf("leetcode: %s", res.Status)
	}
	var r response
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return p, err
	}
	if r.Data.MatchedUser == nil {
		return p, fmt.Errorf("leetcode: no such user %q", handle)
	}

	by := func(list []bucket, d string) bucket {
		for _, b := range list {
			if b.Difficulty == d {
				return b
			}
		}
		return bucket{}
	}
	ac := r.Data.MatchedUser.SubmitStats.AcSubmissionNum
	total := r.Data.MatchedUser.SubmitStats.TotalSubmissionNum

	p = model.Platform{
		Handle:     handle,
		URL:        "https://leetcode.com/u/" + handle,
		Solved:     by(ac, "All").Count,
		Acceptance: round1(100 * float64(by(ac, "All").Submissions) / float64(max(1, by(total, "All").Submissions))),
	}
	p.Histogram = raw([][2]any{
		{"easy", by(ac, "Easy").Count},
		{"medium", by(ac, "Medium").Count},
		{"hard", by(ac, "Hard").Count},
	})

	if c := r.Data.UserContestRanking; c != nil {
		p.Current = int(math.Round(c.Rating))
		p.Contests = c.AttendedContestsCount
		p.GlobalRank = c.GlobalRanking
		p.TopPercent = round2(c.TopPercentage)
		if c.Badge != nil {
			p.Badge = c.Badge.Name
		}
	}

	peak := 0.0
	for _, h := range r.Data.History {
		if !h.Attended {
			continue
		}
		p.Ratings = append(p.Ratings, int(math.Round(h.Rating)))
		if h.Rating > peak {
			peak = h.Rating
			p.PeakContest = h.Contest.Title
			p.PeakDate = time.Unix(h.Contest.StartTime, 0).UTC().Format("Jan 2006")
		}
		if p.First == "" {
			p.First = time.Unix(h.Contest.StartTime, 0).UTC().Format("Jan 2006")
		}
		p.Last = time.Unix(h.Contest.StartTime, 0).UTC().Format("Jan 2006")
	}
	p.Peak = int(math.Round(peak))
	p.Bands = raw([][2]any{{1600, ""}, {1900, ""}})
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
func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }
