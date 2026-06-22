package main

import (
	"sort"
	"testing"
)

func TestScoreSearchResultRanksFirefox(t *testing.T) {
	results := []searchResult{
		{AttrName: "firefox-bin", PName: "firefox-bin"},
		{AttrName: "firefox", PName: "firefox"},
		{AttrName: "librewolf", PName: "librewolf", Description: "Firefox fork"},
	}
	for i := range results {
		results[i].Score = scoreSearchResult("firefox", results[i])
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })

	if results[0].AttrName != "firefox" || results[1].AttrName != "firefox-bin" || results[2].AttrName != "librewolf" {
		t.Fatalf("bad order: %#v", results)
	}
}
