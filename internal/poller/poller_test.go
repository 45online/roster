package poller

import (
	"reflect"
	"testing"

	gh "github.com/45online/roster/internal/adapters/github"
)

func TestFreshEventsOldestFirst_NoCursor_ReturnsAllReversed(t *testing.T) {
	events := []gh.Event{
		{ID: "3"},
		{ID: "2"},
		{ID: "1"},
	}
	got := freshEventsOldestFirst(events, "")
	want := []gh.Event{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFreshEventsOldestFirst_CursorMatch_ReturnsOnlyNewer(t *testing.T) {
	// GitHub returns newest-first: 5,4,3,2,1
	// We've seen "2", so 3,4,5 are fresh; in oldest-first that's 3,4,5.
	events := []gh.Event{
		{ID: "5"},
		{ID: "4"},
		{ID: "3"},
		{ID: "2"},
		{ID: "1"},
	}
	got := freshEventsOldestFirst(events, "2")
	want := []gh.Event{{ID: "3"}, {ID: "4"}, {ID: "5"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFreshEventsOldestFirst_CursorNotFound_ReturnsAll(t *testing.T) {
	// Cursor "99" isn't in the feed (probably aged out). Behavior:
	// treat all events as fresh.
	events := []gh.Event{
		{ID: "5"},
		{ID: "4"},
	}
	got := freshEventsOldestFirst(events, "99")
	want := []gh.Event{{ID: "4"}, {ID: "5"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFreshEventsOldestFirst_AllSeen_ReturnsEmpty(t *testing.T) {
	events := []gh.Event{{ID: "5"}, {ID: "4"}}
	got := freshEventsOldestFirst(events, "5")
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestFreshEventsOldestFirst_Empty(t *testing.T) {
	got := freshEventsOldestFirst(nil, "anything")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
