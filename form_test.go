package liveview

import (
	"net/url"
	"reflect"
	"testing"
)

func TestDecodeForm(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  map[string]any
	}{
		{
			name:  "flat scalar",
			query: "name=Ada&age=36",
			want:  map[string]any{"name": "Ada", "age": "36"},
		},
		{
			name:  "single nesting",
			query: "user[name]=Ada&user[email]=ada@x.io",
			want: map[string]any{
				"user": map[string]any{"name": "Ada", "email": "ada@x.io"},
			},
		},
		{
			name:  "deep nesting",
			query: "user[address][city]=Bath&user[address][zip]=BA1",
			want: map[string]any{
				"user": map[string]any{
					"address": map[string]any{"city": "Bath", "zip": "BA1"},
				},
			},
		},
		{
			name:  "list accumulation",
			query: "tags[]=a&tags[]=b&tags[]=c",
			want:  map[string]any{"tags": []string{"a", "b", "c"}},
		},
		{
			name:  "nested list",
			query: "user[roles][]=admin&user[roles][]=ops",
			want: map[string]any{
				"user": map[string]any{"roles": []string{"admin", "ops"}},
			},
		},
		{
			name:  "last scalar wins",
			query: "x=1&x=2",
			want:  map[string]any{"x": "2"},
		},
		{
			name:  "mixed",
			query: "user[name]=Ada&tags[]=go&user[age]=36",
			want: map[string]any{
				"user": map[string]any{"name": "Ada", "age": "36"},
				"tags": []string{"go"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeFormString(tc.query)
			if err != nil {
				t.Fatalf("DecodeFormString: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("DecodeForm(%q)\n got %#v\nwant %#v", tc.query, got, tc.want)
			}
		})
	}
}

func TestDecodeFormDirectValues(t *testing.T) {
	v := url.Values{"user[name]": {"Grace"}}
	got := DecodeForm(v)
	want := map[string]any{"user": map[string]any{"name": "Grace"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DecodeForm=%#v want %#v", got, want)
	}
}

func TestForm(t *testing.T) {
	f := NewForm("user", map[string]any{"name": "Ada", "email": ""})
	if f.GetString("name") != "Ada" {
		t.Errorf("GetString=%q", f.GetString("name"))
	}
	if f.Get("missing") != nil {
		t.Error("missing field should be nil")
	}
	if f.InputName("email") != "user[email]" {
		t.Errorf("InputName=%q", f.InputName("email"))
	}
	if !f.Valid() {
		t.Error("fresh form should be valid")
	}
	f.AddError("email", "can't be blank")
	f.AddError("email", "is required")
	if f.Valid() || !f.HasErrors() {
		t.Error("form with errors should be invalid")
	}
	if got := f.Errors("email"); !reflect.DeepEqual(got, []string{"can't be blank", "is required"}) {
		t.Errorf("Errors=%v (order matters)", got)
	}
	if f.Errors("name") != nil {
		t.Error("clean field should have nil errors")
	}
}

func TestFormEmptyNameInputName(t *testing.T) {
	f := NewForm("", nil)
	if f.InputName("q") != "q" {
		t.Errorf("InputName with empty form name=%q want q", f.InputName("q"))
	}
}

func BenchmarkDecodeForm(b *testing.B) {
	v := url.Values{
		"user[name]":       {"Ada"},
		"user[address][c]": {"Bath"},
		"tags[]":           {"a", "b", "c"},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = DecodeForm(v)
	}
}
