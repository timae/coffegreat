package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Views struct {
	pages map[string]*template.Template
}

// V is the data passed to every template.
type V map[string]any

// assetVersion busts browser caches on each deploy.
var assetVersion = strconv.FormatInt(time.Now().Unix(), 36)

func loadViews() (*Views, error) {
	files, err := fs.Glob(webFS, "web/templates/*.html")
	if err != nil {
		return nil, err
	}
	v := &Views{pages: map[string]*template.Template{}}
	for _, f := range files {
		name := strings.TrimSuffix(path.Base(f), ".html")
		if name == "layout" || name == "partials" {
			continue
		}
		t, err := template.New(name).Funcs(funcs).ParseFS(webFS,
			"web/templates/layout.html", "web/templates/partials.html", f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		v.pages[name] = t
	}
	return v, nil
}

func (app *App) render(w http.ResponseWriter, r *http.Request, status int, page string, data V) {
	t, ok := app.views.pages[page]
	if !ok {
		http.Error(w, "unknown page "+page, http.StatusInternalServerError)
		return
	}
	if data == nil {
		data = V{}
	}
	data["Authed"] = app.auth.valid(r) && page != "login"
	data["AuthEnabled"] = !app.auth.disabled
	data["Path"] = r.URL.Path
	data["Asset"] = assetVersion
	if _, ok := data["Flash"]; !ok {
		data["Flash"] = popFlash(w, r)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Printf("render %s: %v", page, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// ---------- flash messages (one-shot cookie) ----------

func setFlash(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{Name: "cg_flash", Value: url.QueryEscape(msg), Path: "/", MaxAge: 30,
		HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func popFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie("cg_flash")
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: "cg_flash", Value: "", Path: "/", MaxAge: -1})
	s, _ := url.QueryUnescape(c.Value)
	return s
}

// ---------- template helpers ----------

var funcs = template.FuncMap{
	"icon":      icon,
	"num":       num,
	"num1":      num1,
	"has":       has,
	"ival":      ival,
	"fval":      fval,
	"isID":      isID,
	"ratio":     fmtRatio,
	"date":      fmtDate,
	"dateInput": dateInput,
	"dtInput":   func(t time.Time) string { return t.Local().Format("2006-01-02T15:04") },
	"ago":       ago,
	"clock":     func(t time.Time) string { return t.Local().Format("15:04") },
	"dayKey":    func(t time.Time) string { return t.Local().Format("2006-01-02") },
	"dayLabel":  dayLabel,
	"shortDay": func(t time.Time) string {
		if t.Local().Format("2006-01-02") == time.Now().Local().Format("2006-01-02") {
			return ""
		}
		return t.Local().Format("2 Jan")
	},
	"label":     labelArt,
	"radar":     radar,
	"swatch":    swatch,
	"pulseBars": pulseBars,
	"seq":       seq,
	"add":       func(a, b int) int { return a + b },
	"sub6":      func(n int) int { return 6 - n },
	"lower":     strings.ToLower,
	"eqi":       strings.EqualFold,
	"pct":       pct,
	"dict":      dict,
	"taste":     tasteLabel,
	"tasteHint": tasteHint,
	"ratioName": ratioName,
	"greeting":  greeting,
	"today":     func() string { return time.Now().Local().Format("Monday, 2 January") },
	"opts": func(name string) []string {
		switch name {
		case "burr":
			return BurrTypes
		case "method":
			return BrewMethods
		case "process":
			return Processes
		case "roast":
			return RoastLevels
		case "flavors":
			return FlavorIdeas
		}
		return nil
	},
}

func deref(v any) (any, bool) {
	if v == nil {
		return nil, false
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	return rv.Interface(), true
}

func has(v any) bool { _, ok := deref(v); return ok }

func toFloat(v any) (float64, bool) {
	x, ok := deref(v)
	if !ok {
		return 0, false
	}
	switch n := x.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}

// num formats a number compactly: 18 → "18", 18.5 → "18.5". nil → "".
func num(v any) string {
	f, ok := toFloat(v)
	if !ok {
		return ""
	}
	return strconv.FormatFloat(math.Round(f*10)/10, 'f', -1, 64)
}

// num1 always shows one decimal.
func num1(v any) string {
	f, ok := toFloat(v)
	if !ok {
		return ""
	}
	return strconv.FormatFloat(f, 'f', 1, 64)
}

func ival(v any) int64 {
	f, _ := toFloat(v)
	return int64(math.Round(f))
}

func fval(v any) float64 { f, _ := toFloat(v); return f }

// isID reports whether a nullable id equals id.
func isID(p *int64, id int64) bool { return p != nil && *p == id }

func fmtRatio(r float64) string {
	if r <= 0 {
		return "–"
	}
	return "1:" + strconv.FormatFloat(math.Round(r*10)/10, 'f', 1, 64)
}

func ratioName(r float64, method string) string {
	if r <= 0 {
		return ""
	}
	m := strings.ToLower(method)
	if m != "" && m != "espresso" {
		switch {
		case r < 12:
			return "strong"
		case r <= 17:
			return "classic"
		default:
			return "light"
		}
	}
	switch {
	case r < 1.5:
		return "ristretto"
	case r <= 2.5:
		return "normale"
	case r <= 4:
		return "lungo"
	default:
		return "allongé"
	}
}

func fmtDate(v any) string {
	x, ok := deref(v)
	if !ok {
		return ""
	}
	t, ok := x.(time.Time)
	if !ok {
		return ""
	}
	return t.Local().Format("2 Jan 2006")
}

func dateInput(v any) string {
	x, ok := deref(v)
	if !ok {
		return ""
	}
	if t, ok := x.(time.Time); ok {
		return t.Format("2006-01-02")
	}
	return ""
}

func ago(v any) string {
	x, ok := deref(v)
	if !ok {
		return "never"
	}
	t := x.(time.Time)
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Local().Format("2 Jan 2006")
}

func dayLabel(t time.Time) string {
	t = t.Local()
	now := time.Now().Local()
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return "Today"
	}
	yd := now.AddDate(0, 0, -1)
	y3, m3, d3 := yd.Date()
	if y1 == y3 && m1 == m3 && d1 == d3 {
		return "Yesterday"
	}
	if y1 == y2 {
		return t.Format("Monday, 2 January")
	}
	return t.Format("2 January 2006")
}

func greeting() string {
	h := time.Now().Local().Hour()
	switch {
	case h < 5:
		return "Late night pull?"
	case h < 11:
		return "Good morning."
	case h < 14:
		return "Midday cup."
	case h < 18:
		return "Afternoon shot."
	}
	return "Evening, barista."
}

func seq(a, b int) []int {
	var out []int
	for i := a; i <= b; i++ {
		out = append(out, i)
	}
	return out
}

func pct(v any, max float64) float64 {
	f, _ := toFloat(v)
	if max == 0 {
		return 0
	}
	p := f / max * 100
	return math.Max(0, math.Min(100, p))
}

func dict(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[fmt.Sprint(kv[i])] = kv[i+1]
	}
	return m
}

func tasteLabel(p *int64) string {
	if p == nil {
		return ""
	}
	switch *p {
	case -2:
		return "very sour"
	case -1:
		return "a bit sour"
	case 0:
		return "balanced"
	case 1:
		return "a bit bitter"
	case 2:
		return "very bitter"
	}
	return ""
}

func tasteHint(p *int64) string {
	if p == nil {
		return ""
	}
	switch {
	case *p < 0:
		return "Under-extracted → grind finer, raise temp or pull longer."
	case *p > 0:
		return "Over-extracted → grind coarser, lower temp or pull shorter."
	}
	return "Dialed in. Write it down."
}

// ---------- icons (inline SVG, stroke-based) ----------

var iconPaths = map[string]string{
	"home":    `<path d="M3 10.5 12 3l9 7.5"/><path d="M5 9v12h14V9"/><path d="M10 21v-6h4v6"/>`,
	"bean":    `<ellipse cx="12" cy="12" rx="6" ry="9" transform="rotate(35 12 12)"/><path d="M7.6 17.2c3-1.6 2-5.2 4.4-7.4 1.4-1.3 3-1.6 4.4-2.9"/>`,
	"grinder": `<path d="M7 3h10l-2 6H9z"/><rect x="6" y="9" width="12" height="10" rx="2"/><path d="M12 13v2"/><path d="M8 21h8"/>`,
	"cup":     `<path d="M4 9h13v4a6 6 0 0 1-6 6h-1a6 6 0 0 1-6-6z"/><path d="M17 10.5h1.5a2.5 2.5 0 0 1 0 5H16.5"/><path d="M8 2.5c-.6 1 .6 1.8 0 3M12 2.5c-.6 1 .6 1.8 0 3"/>`,
	"dial":    `<path d="M4 6h9M17 6h3M4 12h3M11 12h9M4 18h11M19 18h1"/><circle cx="15" cy="6" r="2"/><circle cx="9" cy="12" r="2"/><circle cx="17" cy="18" r="2"/>`,
	"plus":    `<path d="M12 5v14M5 12h14"/>`,
	"edit":    `<path d="M4 20h4L19 9l-4-4L4 16z"/><path d="M13.5 6.5l4 4"/>`,
	"trash":   `<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/>`,
	"logout":  `<path d="M15 4h4v16h-4"/><path d="M10 16l-4-4 4-4"/><path d="M6 12h11"/>`,
	"timer":   `<circle cx="12" cy="13" r="8"/><path d="M12 9v4l2.5 2.5M9.5 2h5"/>`,
	"thermo":  `<path d="M10 14.5V5a2 2 0 1 1 4 0v9.5a4 4 0 1 1-4 0z"/><path d="M12 11v6"/>`,
	"drop":    `<path d="M12 3s6 6.5 6 11a6 6 0 0 1-12 0c0-4.5 6-11 6-11z"/>`,
	"scale":   `<path d="M4 20h16l-2-11H6z"/><circle cx="12" cy="5" r="2"/><path d="M12 7v2"/>`,
	"arrow":   `<path d="M5 12h14M13 6l6 6-6 6"/>`,
	"back":    `<path d="M19 12H5M11 6l-6 6 6 6"/>`,
	"search":  `<circle cx="11" cy="11" r="7"/><path d="M20 20l-3.5-3.5"/>`,
	"repeat":  `<path d="M17 2l4 4-4 4"/><path d="M3 11V9a3 3 0 0 1 3-3h15"/><path d="M7 22l-4-4 4-4"/><path d="M21 13v2a3 3 0 0 1-3 3H3"/>`,
	"heart":   `<path d="M12 20s-7.5-4.4-7.5-10A4.3 4.3 0 0 1 12 7.3 4.3 4.3 0 0 1 19.5 10c0 5.6-7.5 10-7.5 10z"/>`,
	"flame":   `<path d="M12 21a6 6 0 0 0 6-6c0-4-3-6-4-10-2 2-3 3.5-3 6-1-1-1.5-2-1.5-3C7 10 6 12.5 6 15a6 6 0 0 0 6 6z"/>`,
	"pin":     `<path d="M12 21s7-6.2 7-12a7 7 0 0 0-14 0c0 5.8 7 12 7 12z"/><circle cx="12" cy="9" r="2.5"/>`,
	"spark":   `<path d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M5.6 18.4l2.8-2.8M15.6 8.4l2.8-2.8"/>`,
}

func icon(name string) template.HTML {
	p, ok := iconPaths[name]
	if !ok {
		return ""
	}
	return template.HTML(`<svg class="i" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">` + p + `</svg>`)
}
