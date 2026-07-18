package liveview

import (
	"bytes"
	"errors"
	"sort"
)

// UploadConfig describes a named upload slot declared by a view via
// [Socket.AllowUpload]. It bounds what the client may send and holds the
// in-flight [UploadEntry] values as their chunks arrive over the socket.
type UploadConfig struct {
	// Name is the upload's identifier, referenced by the client and by
	// [ConsumeUploadedEntries].
	Name string
	// Accept is an optional list of acceptable extensions or MIME types
	// (advisory; enforcement is best-effort and client-side).
	Accept []string
	// MaxEntries caps how many files may be uploaded at once (0 means 1).
	MaxEntries int
	// MaxFileSize caps the byte size of a single entry (0 means unlimited).
	MaxFileSize int64

	entries map[string]*UploadEntry
}

// UploadEntry is a single file being uploaded. The client announces it (name,
// size, type) and then streams its bytes in chunks; the server assembles them
// and tracks progress until the entry is complete.
type UploadEntry struct {
	// Ref is the client-assigned reference identifying this entry.
	Ref string
	// Name is the original file name.
	Name string
	// Size is the declared total size in bytes.
	Size int64
	// Type is the declared MIME type.
	Type string
	// Progress is the percentage received so far, 0..100.
	Progress int
	// Done reports whether all bytes have been received.
	Done bool

	buf bytes.Buffer
}

// Bytes returns the assembled content received so far. For a completed entry
// this is the full file.
func (e *UploadEntry) Bytes() []byte { return e.buf.Bytes() }

// newUploadConfig applies option defaults.
func newUploadConfig(name string, opts UploadOptions) *UploadConfig {
	max := opts.MaxEntries
	if max <= 0 {
		max = 1
	}
	return &UploadConfig{
		Name:        name,
		Accept:      opts.Accept,
		MaxEntries:  max,
		MaxFileSize: opts.MaxFileSize,
		entries:     make(map[string]*UploadEntry),
	}
}

// UploadOptions configures an upload slot at [Socket.AllowUpload] time.
type UploadOptions struct {
	// Accept lists acceptable extensions or MIME types (advisory).
	Accept []string
	// MaxEntries caps concurrent files (defaults to 1 when <= 0).
	MaxEntries int
	// MaxFileSize caps a single entry's byte size (0 means unlimited).
	MaxFileSize int64
}

// Entries returns the current upload entries in a stable order (by Ref), so a
// view's Render is deterministic across calls.
func (u *UploadConfig) Entries() []*UploadEntry {
	out := make([]*UploadEntry, 0, len(u.entries))
	for _, e := range u.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

// register creates (or returns) the entry for ref, announcing its metadata.
func (u *UploadConfig) register(ref, name string, size int64, mime string) (*UploadEntry, error) {
	if e, ok := u.entries[ref]; ok {
		return e, nil
	}
	if len(u.entries) >= u.MaxEntries {
		return nil, errors.New("liveview: too many upload entries")
	}
	if u.MaxFileSize > 0 && size > u.MaxFileSize {
		return nil, errors.New("liveview: upload exceeds max file size")
	}
	e := &UploadEntry{Ref: ref, Name: name, Size: size, Type: mime}
	u.entries[ref] = e
	return e, nil
}

// appendChunk appends data to the entry identified by ref, updates its progress,
// and marks it done when last is true or the declared size is reached.
func (u *UploadConfig) appendChunk(ref string, data []byte, last bool) (*UploadEntry, error) {
	e, ok := u.entries[ref]
	if !ok {
		return nil, errors.New("liveview: unknown upload ref")
	}
	if u.MaxFileSize > 0 && int64(e.buf.Len()+len(data)) > u.MaxFileSize {
		return nil, errors.New("liveview: upload exceeds max file size")
	}
	e.buf.Write(data)
	if e.Size > 0 {
		p := int(int64(e.buf.Len()) * 100 / e.Size)
		if p > 100 {
			p = 100
		}
		e.Progress = p
	}
	if last || (e.Size > 0 && int64(e.buf.Len()) >= e.Size) {
		e.Done = true
		e.Progress = 100
	}
	return e, nil
}

// consume passes each completed entry to fn and removes it from the config,
// returning the collected results. Incomplete entries are left untouched.
func (u *UploadConfig) consume(fn func(*UploadEntry) any) []any {
	done := make([]*UploadEntry, 0)
	for _, e := range u.entries {
		if e.Done {
			done = append(done, e)
		}
	}
	sort.Slice(done, func(i, j int) bool { return done[i].Ref < done[j].Ref })
	out := make([]any, 0, len(done))
	for _, e := range done {
		out = append(out, fn(e))
		delete(u.entries, e.Ref)
	}
	return out
}
