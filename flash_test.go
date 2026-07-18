package liveview

import (
	"reflect"
	"testing"
)

func TestFlashBasics(t *testing.T) {
	f := NewFlash()
	if f.Has("info") {
		t.Fatal("new flash should be empty")
	}
	f.Put("info", "saved")
	f.Put("error", "nope")
	if got := f.Get("info"); got != "saved" {
		t.Errorf("Get(info)=%q want saved", got)
	}
	if !f.Has("error") {
		t.Error("Has(error) should be true")
	}
	if got := f.Kinds(); !reflect.DeepEqual(got, []string{"error", "info"}) {
		t.Errorf("Kinds()=%v want [error info] (sorted)", got)
	}
	f.Delete("error")
	if f.Has("error") {
		t.Error("Delete did not remove error")
	}
	f.Clear()
	if len(f) != 0 {
		t.Errorf("Clear left %d entries", len(f))
	}
}

func TestFlashMerge(t *testing.T) {
	a := Flash{"info": "a", "warn": "keep"}
	b := Flash{"info": "b", "error": "e"}
	a.Merge(b)
	want := Flash{"info": "b", "warn": "keep", "error": "e"}
	if !reflect.DeepEqual(a, want) {
		t.Errorf("Merge=%v want %v", a, want)
	}
}

func TestFlashSocketIntegration(t *testing.T) {
	s := NewSocket()
	if GetFlash(s, "info") != "" {
		t.Error("empty socket should have no flash")
	}
	PutFlash(s, "info", "welcome")
	if !s.Changed(flashAssignKey) {
		t.Error("PutFlash should mark the flash assign changed")
	}
	if got := GetFlash(s, "info"); got != "welcome" {
		t.Errorf("GetFlash=%q want welcome", got)
	}
	// Flash rides along in the assigns snapshot.
	snap := s.Assigns()
	fl, ok := snap[flashAssignKey].(Flash)
	if !ok || fl.Get("info") != "welcome" {
		t.Errorf("flash not present in assigns snapshot: %#v", snap[flashAssignKey])
	}

	s.ResetChanges()
	ClearFlash(s)
	if GetFlash(s, "info") != "" {
		t.Error("ClearFlash did not clear")
	}
	if !s.Changed(flashAssignKey) {
		t.Error("ClearFlash should mark changed")
	}
}

func TestSocketFlashIdempotentAttach(t *testing.T) {
	s := NewSocket()
	f1 := SocketFlash(s)
	f1.Put("info", "x")
	f2 := SocketFlash(s)
	if f2.Get("info") != "x" {
		t.Error("SocketFlash should return the same attached map")
	}
}
