package main

import (
	"strings"
	"testing"
)

func TestPetPhrasesHaveChoices(t *testing.T) {
	for action, phrases := range petPhrases {
		if len(phrases) < 2 || len(phrases) > 3 {
			t.Fatalf("%s has %d phrases, want 2 or 3", action, len(phrases))
		}
		petSay(action)
		found := false
		for _, phrase := range phrases {
			if tuiPetSpeech == phrase {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s produced an unknown phrase %q", action, tuiPetSpeech)
		}
	}
}

func TestPetReactionsRender(t *testing.T) {
	for _, reaction := range petReactions {
		if !strings.Contains(petText(reaction), reaction) {
			t.Fatalf("pet does not render reaction %q", reaction)
		}
	}
}
