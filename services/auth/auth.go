// Package auth signs people in through Forgejo (OAuth2 authorization code + PKCE)
// and keeps who they are in an HMAC-signed session cookie. Access is decided by
// models/member on every request, so removing a member takes effect at once.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
)

const (
	sessionCookie = "dash_session"
	oauthCookie   = "dash_oauth"
	sessionTTL    = 7 * 24 * time.Hour
	oauthTTL      = 10 * time.Minute
)

type session struct {
	Login  string `json:"l"`
	Avatar string `json:"a,omitempty"`
	Exp    int64  `json:"e"`
}

func sign(payload []byte) string {
	m := hmac.New(sha256.New, []byte(setting.SessionSecret))
	m.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func verify(value string) ([]byte, bool) {
	p, s, ok := strings.Cut(value, ".")
	if !ok {
		return nil, false
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(p)
	sig, err2 := base64.RawURLEncoding.DecodeString(s)
	if err1 != nil || err2 != nil {
		return nil, false
	}
	m := hmac.New(sha256.New, []byte(setting.SessionSecret))
	m.Write(payload)
	return payload, hmac.Equal(sig, m.Sum(nil))
}

// cookiePath keeps the cookies to this app when it lives under a sub-path of a shared host.
func cookiePath() string { return setting.AppSubURL + "/" }

func setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: cookiePath(), MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: setting.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: cookiePath(), MaxAge: -1, HttpOnly: true, Secure: setting.CookieSecure, SameSite: http.SameSiteLaxMode})
}

// SignIn starts a session for login.
func SignIn(w http.ResponseWriter, login, avatar string) {
	b, _ := json.Marshal(session{Login: login, Avatar: avatar, Exp: time.Now().Add(sessionTTL).Unix()})
	setCookie(w, sessionCookie, sign(b), sessionTTL)
}

// SignOut ends the session.
func SignOut(w http.ResponseWriter) { clearCookie(w, sessionCookie) }

// SessionCookie is the cookie for login, for tests.
func SessionCookie(login string) *http.Cookie {
	b, _ := json.Marshal(session{Login: login, Exp: time.Now().Add(time.Hour).Unix()})
	return &http.Cookie{Name: sessionCookie, Value: sign(b)}
}

// Doer returns the signed-in member, or nil when there is no valid session or the member lost access.
func Doer(r *http.Request) (*member_model.Member, string) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, ""
	}
	b, ok := verify(c.Value)
	var s session
	if !ok || json.Unmarshal(b, &s) != nil || s.Login == "" || time.Now().Unix() > s.Exp {
		return nil, ""
	}
	m, err := member_model.GetByLogin(r.Context(), s.Login)
	if err != nil || member_model.RoleLevel(m.Role) == 0 {
		return nil, ""
	}
	return m, s.Avatar
}

// SafeNext keeps redirects on this site.
func SafeNext(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") && !strings.HasPrefix(next, "/\\") {
		return next
	}
	return "/admin"
}

type oauthState struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
	Next     string `json:"n"`
}

func redirectURI() string { return setting.BaseURL + "/auth/callback" }

// AuthorizeURL remembers the state in a cookie and returns the Forgejo authorization URL.
func AuthorizeURL(w http.ResponseWriter, next string) string {
	st := oauthState{State: rand.Text(), Verifier: rand.Text() + rand.Text(), Next: SafeNext(next)}
	b, _ := json.Marshal(st)
	setCookie(w, oauthCookie, sign(b), oauthTTL)
	sum := sha256.Sum256([]byte(st.Verifier))
	q := url.Values{
		"client_id":             {setting.OAuthClientID},
		"redirect_uri":          {redirectURI()},
		"response_type":         {"code"},
		"scope":                 {"openid profile"},
		"state":                 {st.State},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	return setting.ForgejoURL + "/login/oauth/authorize?" + q.Encode()
}

// ErrSignIn is a sign-in failure the user should see.
type ErrSignIn struct{ Reason string }

func (err ErrSignIn) Error() string { return err.Reason }

var client = &http.Client{Timeout: 15 * time.Second}

// Callback checks the state and exchanges the code; it returns the Forgejo login,
// avatar and where to go next.
func Callback(w http.ResponseWriter, r *http.Request) (login, avatar, next string, err error) {
	c, cerr := r.Cookie(oauthCookie)
	if cerr != nil {
		return "", "", "", ErrSignIn{Reason: "сессия входа истекла, попробуйте ещё раз"}
	}
	clearCookie(w, oauthCookie)
	b, ok := verify(c.Value)
	var st oauthState
	if !ok || json.Unmarshal(b, &st) != nil || st.State == "" || r.URL.Query().Get("state") != st.State {
		return "", "", "", ErrSignIn{Reason: "неверный state, попробуйте ещё раз"}
	}
	if e := r.URL.Query().Get("error"); e != "" {
		return "", "", "", ErrSignIn{Reason: "Forgejo отклонил вход: " + e}
	}
	login, avatar, err = exchange(r.Context(), r.URL.Query().Get("code"), st.Verifier)
	return login, avatar, st.Next, err
}

func exchange(ctx context.Context, code, verifier string) (login, avatar string, err error) {
	if code == "" {
		return "", "", errors.New("empty code")
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI()},
		"client_id":     {setting.OAuthClientID},
		"client_secret": {setting.OAuthClientSecret},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, setting.ForgejoURL+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := doJSON(req, &tok); err != nil {
		return "", "", fmt.Errorf("token: %w", err)
	}
	if tok.AccessToken == "" {
		return "", "", errors.New("token: empty access_token")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, setting.ForgejoURL+"/login/oauth/userinfo", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	var info struct {
		PreferredUsername string `json:"preferred_username"`
		Picture           string `json:"picture"`
	}
	if err := doJSON(req, &info); err != nil {
		return "", "", fmt.Errorf("userinfo: %w", err)
	}
	if info.PreferredUsername == "" {
		return "", "", errors.New("userinfo: no preferred_username")
	}
	return strings.ToLower(info.PreferredUsername), info.Picture, nil
}

func doJSON(req *http.Request, v any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return json.Unmarshal(body, v)
}
