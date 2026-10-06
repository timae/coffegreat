package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookie = "cg_session"
	sessionTTL    = 30 * 24 * time.Hour
)

// Auth is a single-user password gate with a stateless, HMAC-signed cookie.
type Auth struct {
	password []byte // sha256 of APP_PASSWORD
	secret   []byte
	disabled bool
}

func newAuth() (*Auth, error) {
	pw := os.Getenv("APP_PASSWORD")
	if pw == "" {
		if os.Getenv("DEV") == "1" {
			log.Println("WARNING: APP_PASSWORD not set — auth disabled (DEV=1)")
			return &Auth{disabled: true}, nil
		}
		return nil, errors.New("APP_PASSWORD must be set (or DEV=1 for local development)")
	}
	pwHash := sha256.Sum256([]byte(pw))
	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		// Derive from the password so sessions survive restarts and work
		// across replicas without extra config. Changing the password logs
		// everyone out — which is what you'd want anyway.
		secret = "coffee-grate:" + pw
	}
	s := sha256.Sum256([]byte(secret))
	return &Auth{password: pwHash[:], secret: s[:]}, nil
}

func (a *Auth) check(pw string) bool {
	h := sha256.Sum256([]byte(pw))
	return subtle.ConstantTimeCompare(h[:], a.password) == 1
}

func (a *Auth) sign(payload string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (a *Auth) issue(w http.ResponseWriter, r *http.Request) {
	exp := strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 10)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    exp + "." + a.sign(exp),
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *Auth) clear(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
}

func (a *Auth) valid(r *http.Request) bool {
	if a.disabled {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	exp, sig, ok := strings.Cut(c.Value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(a.sign(exp))) {
		return false
	}
	ts, err := strconv.ParseInt(exp, 10, 64)
	return err == nil && time.Now().Unix() < ts
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// middleware: auth gate, same-origin check for writes, security headers.
func (app *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src https://fonts.gstatic.com; script-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'self'")

		if r.Method == http.MethodPost && !sameOrigin(r) {
			http.Error(w, "cross-origin request blocked", http.StatusForbidden)
			return
		}

		p := r.URL.Path
		public := p == "/login" || p == "/healthz" || strings.HasPrefix(p, "/static/")
		if !public && !app.auth.valid(r) {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		// Browsers always send Origin on POST; absence means a non-browser
		// client, which can't carry our SameSite cookie cross-site anyway.
		return origin == ""
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host || u.Host == r.Header.Get("X-Forwarded-Host")
}
