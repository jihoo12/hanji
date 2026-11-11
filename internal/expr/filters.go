package expr

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Filters is the global filter table. Other packages may add to it at init.
var Filters = map[string]Filter{
	"upper": func(in any, _ []any) (any, error) { return strings.ToUpper(Str(in)), nil },
	"lower": func(in any, _ []any) (any, error) { return strings.ToLower(Str(in)), nil },
	// format("x"): fill the %s placeholder of a string
	"format": func(in any, a []any) (any, error) {
		s := Str(in)
		for _, x := range a {
			s = strings.Replace(s, "%s", Str(x), 1)
		}
		return s, nil
	},
	"trim": func(in any, _ []any) (any, error) { return strings.TrimSpace(Str(in)), nil },
	"raw":  func(in any, _ []any) (any, error) { return HTML(Str(in)), nil },
	"escape": func(in any, _ []any) (any, error) {
		return html.EscapeString(Str(in)), nil
	},
	"strip": func(in any, _ []any) (any, error) { return StripTags(Str(in)), nil },
	"default": func(in any, a []any) (any, error) {
		if !Truthy(in) && len(a) > 0 {
			return a[0], nil
		}
		return in, nil
	},
	"json": func(in any, _ []any) (any, error) {
		b, err := json.Marshal(in)
		return HTML(b), err
	},
	"len": func(in any, _ []any) (any, error) { return float64(Len(in)), nil },
	"date": func(in any, a []any) (any, error) {
		t, ok := in.(time.Time)
		if !ok {
			return Str(in), nil
		}
		layout := "YYYY-MM-DD"
		if len(a) > 0 {
			layout = Str(a[0])
		}
		return FormatDate(t, layout), nil
	},
	"truncate": func(in any, a []any) (any, error) {
		n := 120
		if len(a) > 0 {
			if f, ok := Num(a[0]); ok {
				n = int(f)
			}
		}
		r := []rune(Str(in))
		if len(r) <= n {
			return string(r), nil
		}
		return strings.TrimSpace(string(r[:n])) + "…", nil
	},
	"join": func(in any, a []any) (any, error) {
		sep := ", "
		if len(a) > 0 {
			sep = Str(a[0])
		}
		parts := []string{}
		for _, x := range List(in) {
			parts = append(parts, Str(x))
		}
		return strings.Join(parts, sep), nil
	},
	"split": func(in any, a []any) (any, error) {
		sep := ","
		if len(a) > 0 {
			sep = Str(a[0])
		}
		out := []any{}
		for _, s := range strings.Split(Str(in), sep) {
			out = append(out, strings.TrimSpace(s))
		}
		return out, nil
	},
	"first": func(in any, _ []any) (any, error) {
		if l := List(in); len(l) > 0 {
			return l[0], nil
		}
		return nil, nil
	},
	"last": func(in any, _ []any) (any, error) {
		if l := List(in); len(l) > 0 {
			return l[len(l)-1], nil
		}
		return nil, nil
	},
	"reverse": func(in any, _ []any) (any, error) {
		l := List(in)
		out := make([]any, len(l))
		for i, x := range l {
			out[len(l)-1-i] = x
		}
		return out, nil
	},
	"limit": func(in any, a []any) (any, error) {
		l := List(in)
		if len(a) > 0 {
			if f, ok := Num(a[0]); ok && int(f) < len(l) && f >= 0 {
				return l[:int(f)], nil
			}
		}
		return l, nil
	},
	"offset": func(in any, a []any) (any, error) {
		l := List(in)
		if len(a) > 0 {
			if f, ok := Num(a[0]); ok && f >= 0 {
				if int(f) >= len(l) {
					return []any{}, nil
				}
				return l[int(f):], nil
			}
		}
		return l, nil
	},
	// sort("key") or sort("key", "desc"); without a key sorts the values.
	"sort": func(in any, a []any) (any, error) {
		l := append([]any{}, List(in)...)
		key, desc := "", false
		if len(a) > 0 {
			key = Str(a[0])
		}
		if len(a) > 1 {
			desc = Str(a[1]) == "desc"
		}
		sort.SliceStable(l, func(i, j int) bool {
			c := Compare(field(l[i], key), field(l[j], key))
			if desc {
				return c > 0
			}
			return c < 0
		})
		return l, nil
	},
	// where("key") keeps truthy; where("key", v) keeps equal, or containing v
	// when the field is a list (handy for tags).
	"where": func(in any, a []any) (any, error) {
		if len(a) == 0 {
			return nil, fmt.Errorf("where needs a key")
		}
		out := []any{}
		for _, x := range List(in) {
			f := field(x, Str(a[0]))
			keep := false
			switch {
			case len(a) == 1:
				keep = Truthy(f)
			default:
				if fl, ok := f.([]any); ok {
					for _, y := range fl {
						keep = keep || Equal(y, a[1])
					}
				} else {
					keep = Equal(f, a[1])
				}
			}
			if keep {
				out = append(out, x)
			}
		}
		return out, nil
	},
	"map": func(in any, a []any) (any, error) {
		if len(a) == 0 {
			return nil, fmt.Errorf("map needs a key")
		}
		out := []any{}
		for _, x := range List(in) {
			out = append(out, field(x, Str(a[0])))
		}
		return out, nil
	},
	"contains": func(in any, a []any) (any, error) {
		if len(a) == 0 {
			return false, nil
		}
		if l, ok := in.([]any); ok {
			for _, x := range l {
				if Equal(x, a[0]) {
					return true, nil
				}
			}
			return false, nil
		}
		return strings.Contains(Str(in), Str(a[0])), nil
	},
}

func field(v any, key string) any {
	if key == "" {
		return v
	}
	for _, k := range strings.Split(key, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

// List coerces a value to a list; nil becomes empty, scalars a single item.
func List(v any) []any {
	switch v := v.(type) {
	case nil:
		return nil
	case []any:
		return v
	}
	return []any{v}
}

func Len(v any) int {
	switch v := v.(type) {
	case []any:
		return len(v)
	case map[string]any:
		return len(v)
	case string:
		return len([]rune(v))
	case HTML:
		return len([]rune(string(v)))
	}
	return 0
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// StripTags removes markup and unescapes entities.
func StripTags(s string) string {
	return html.UnescapeString(tagRe.ReplaceAllString(s, ""))
}

var dateTokens = []struct{ tok, layout string }{
	{"YYYY", "2006"}, {"YY", "06"}, {"MMMM", "January"}, {"MMM", "Jan"}, {"MM", "01"}, {"M", "1"},
	{"DD", "02"}, {"D", "2"}, {"HH", "15"}, {"mm", "04"}, {"ss", "05"},
}

// FormatDate formats with human tokens: YYYY MM M DD D HH mm ss MMM MMMM.
func FormatDate(t time.Time, layout string) string {
	var b strings.Builder
outer:
	for i := 0; i < len(layout); {
		for _, d := range dateTokens {
			if strings.HasPrefix(layout[i:], d.tok) {
				b.WriteString(t.Format(d.layout))
				i += len(d.tok)
				continue outer
			}
		}
		b.WriteByte(layout[i])
		i++
	}
	return b.String()
}
