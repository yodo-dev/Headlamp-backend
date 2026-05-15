package api

import "testing"

func TestParseConversationContentSections_RealFriends(t *testing.T) {
	content := `HL Parent Conversation- Real Friends, Real Life
A simple guide for helping kids build in-person friendship
Kids need friends.

Start with the heart
You don't need to lecture your kid about loneliness or screen time.
Keep it simple.

Ask one easy question
Pick a relaxed moment and ask:
"Who do you wish you had more time with?"

Try this this week
Ask your kid:
"Who would be fun to spend time with soon?"`

	preamble, sections := parseConversationContentSections(content)
	if len(preamble) == 0 {
		t.Fatal("expected non-empty preamble")
	}
	if len(sections) < 3 {
		t.Fatalf("expected at least 3 sections, got %d", len(sections))
	}
	if sections[0].Heading != "Start with the heart" {
		t.Fatalf("unexpected first heading: %s", sections[0].Heading)
	}
	if sections[1].Heading != "Ask one easy question" {
		t.Fatalf("unexpected second heading: %s", sections[1].Heading)
	}
}

func TestParseConversationContentSections_DigitalRhythmSteps(t *testing.T) {
	content := `HL Parent Conversation- Your Family's Digital Rhythm
A conversation guide for creating healthier tech expectations together
Screens can quietly take over a family's rhythm.

Step 1: Start with your values, not the rules
Before talking about limits, talk about what your family wants more of.

Step 2: Talk about mornings
Mornings set the tone for the day.

Final thought
Your family does not need a perfect tech plan.`

	_, sections := parseConversationContentSections(content)
	if len(sections) < 3 {
		t.Fatalf("expected at least 3 sections, got %d", len(sections))
	}
	if sections[0].Number != 1 || sections[0].Heading != "Start with your values, not the rules" {
		t.Fatalf("unexpected step 1 section: %+v", sections[0])
	}
	if sections[1].Number != 2 || sections[1].Heading != "Talk about mornings" {
		t.Fatalf("unexpected step 2 section: %+v", sections[1])
	}
	if sections[2].Heading != "Final thought" {
		t.Fatalf("unexpected final heading: %+v", sections[2])
	}
}
