package liveview

// Diff is a minimal patch describing how to turn one [Rendered] into another.
// It is a sparse map keyed by dynamic-slot index (as a decimal string), holding
// only the slots whose value changed. This is the whole point of the
// static/dynamic split: statics never travel after the first render, and
// unchanged dynamics are omitted.
//
// Value shapes inside a Diff:
//
//   - string        a changed leaf slot; the new escaped HTML for that slot.
//   - Diff          a changed nested component; the recursive sub-diff.
//   - the "s" key   present only on a full/initial diff, holding []string
//     statics so a fresh client can build the document from scratch.
//
// A Diff marshals to JSON as, for example, {"0":"1","2":{"1":"x"}} — the same
// wire shape LiveView uses. An empty Diff means nothing changed.
type Diff map[string]any

// staticsKey is the reserved slot name carrying the static fragments in a full
// diff.
const staticsKey = "s"

// Empty reports whether the diff carries no changes.
func (d Diff) Empty() bool {
	return len(d) == 0
}

// FullDiff produces the complete patch for an initial render: every dynamic slot
// plus the statics under the "s" key. Sending this to a fresh client lets it
// reconstruct the entire document.
func FullDiff(r *Rendered) Diff {
	d := Diff{}
	if r == nil {
		return d
	}
	d[staticsKey] = r.Statics
	for i, dyn := range r.Dynamics {
		switch v := dyn.(type) {
		case string:
			d[itoa(i)] = v
		case *Rendered:
			d[itoa(i)] = FullDiff(v)
		}
	}
	return d
}

// DiffRendered computes the minimal patch that turns prev into next. Both are
// assumed to be renders of the same view (identical template shape). When prev
// is nil the result is a [FullDiff]. Slots that are equal in both are omitted.
//
// If the two renders have different static shapes (different template
// structure, e.g. a conditional that swapped branches), the whole subtree is
// re-sent as a full diff, because the client's cached statics no longer apply.
func DiffRendered(prev, next *Rendered) Diff {
	if next == nil {
		return Diff{}
	}
	if prev == nil || !staticsEqual(prev.Statics, next.Statics) {
		return FullDiff(next)
	}
	d := Diff{}
	for i, nd := range next.Dynamics {
		var pd any
		if i < len(prev.Dynamics) {
			pd = prev.Dynamics[i]
		}
		switch nv := nd.(type) {
		case string:
			pv, ok := pd.(string)
			if !ok || pv != nv {
				d[itoa(i)] = nv
			}
		case *Rendered:
			pr, _ := pd.(*Rendered)
			sub := DiffRendered(pr, nv)
			if !sub.Empty() {
				d[itoa(i)] = sub
			}
		}
	}
	return d
}

func staticsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// itoa is a tiny non-allocating-for-small-ints integer-to-string used for slot
// keys, avoiding a strconv import churn and keeping keys canonical.
func itoa(i int) string {
	if i >= 0 && i < len(smallInts) {
		return smallInts[i]
	}
	return slowItoa(i)
}

var smallInts = [...]string{
	"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
	"10", "11", "12", "13", "14", "15", "16", "17", "18", "19",
	"20", "21", "22", "23", "24", "25", "26", "27", "28", "29", "30", "31",
}

func slowItoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
