// Package setting holds the global configuration, read from the environment.
package setting

import (
	"errors"
	"net/url"
	"os"
	"slices"
	"strings"
)

var (
	// HTTPAddr is the listen address.
	HTTPAddr = ":3000"
	// DBType is "sqlite" or "postgres".
	DBType = "sqlite"
	// DBDSN is a file path for sqlite or a connection string for postgres.
	DBDSN = "data/dash.db"
	// StaticRoot is where templates/ and public/ are read from in non-bindata builds.
	StaticRoot = "."

	// BaseURL is the external address; the OAuth redirect is BaseURL + "/auth/callback".
	BaseURL = "http://localhost:3000"
	// AppSubURL is the path of BaseURL ("/finance" for https://host/finance, "" at the root).
	// The proxy strips it from requests; links and redirects add it back.
	AppSubURL = ""
	// SessionSecret signs the session cookie.
	SessionSecret = ""
	// CookieSecure sets the Secure attribute; only local plain-HTTP runs turn it off.
	CookieSecure = true

	// ForgejoURL is the Forgejo instance used as the OAuth2 provider.
	ForgejoURL = "https://git.cu3rd.ru"
	// OAuthClientID is the Forgejo OAuth2 application id.
	OAuthClientID = ""
	// OAuthClientSecret is its secret.
	OAuthClientSecret = ""
	// BootstrapOwners are Forgejo logins that are always owners, so access cannot be lost.
	BootstrapOwners []string

	// PasswordLogin and Password enable sign-in with a password for one member,
	// for a stand without a Forgejo OAuth app. Both empty: disabled.
	PasswordLogin = ""
	// Password is the password of PasswordLogin; at least 12 characters.
	Password = ""

	// DevLogin enables password-less sign-in as any listed member. Local runs only.
	DevLogin = false
	// SeedDemo fills an empty database with demo data. Local runs only.
	SeedDemo = false

	// DonationWebhookToken authorizes POST /api/donations; empty disables the endpoint.
	DonationWebhookToken = ""

	// RatesURL is the Bank of Russia daily rates feed, loaded once a day; empty disables it.
	RatesURL = "https://www.cbr.ru/scripts/XML_daily.asp"
)

// Load reads DASH_* environment variables.
func Load() {
	HTTPAddr = env("DASH_HTTP_ADDR", HTTPAddr)
	DBType = env("DASH_DB_TYPE", DBType)
	DBDSN = env("DASH_DB_DSN", DBDSN)
	StaticRoot = env("DASH_STATIC_ROOT", StaticRoot)
	BaseURL = strings.TrimRight(env("DASH_BASE_URL", BaseURL), "/")
	AppSubURL = ""
	if u, err := url.Parse(BaseURL); err == nil {
		AppSubURL = strings.TrimRight(u.Path, "/")
	}
	SessionSecret = env("DASH_SESSION_SECRET", SessionSecret)
	CookieSecure = env("DASH_COOKIE_SECURE", "true") != "false"
	ForgejoURL = strings.TrimRight(env("DASH_FORGEJO_URL", ForgejoURL), "/")
	OAuthClientID = env("DASH_OAUTH_CLIENT_ID", OAuthClientID)
	OAuthClientSecret = env("DASH_OAUTH_CLIENT_SECRET", OAuthClientSecret)
	BootstrapOwners = nil
	for l := range strings.SplitSeq(env("DASH_BOOTSTRAP_OWNERS", ""), ",") {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			BootstrapOwners = append(BootstrapOwners, l)
		}
	}
	PasswordLogin = strings.ToLower(strings.TrimSpace(env("DASH_PASSWORD_LOGIN", PasswordLogin)))
	Password = env("DASH_PASSWORD", Password)
	DevLogin = env("DASH_DEV_LOGIN", "false") == "true"
	SeedDemo = env("DASH_SEED_DEMO", "false") == "true"
	DonationWebhookToken = env("DASH_DONATION_WEBHOOK_TOKEN", DonationWebhookToken)
	if v, ok := os.LookupEnv("DASH_RATES_URL"); ok {
		RatesURL = strings.TrimSpace(v)
	}
}

// PasswordLoginEnabled reports whether password sign-in is configured.
func PasswordLoginEnabled() bool { return PasswordLogin != "" && Password != "" }

// OAuthEnabled reports whether sign-in through Forgejo is configured.
func OAuthEnabled() bool { return OAuthClientID != "" && OAuthClientSecret != "" }

// Validate reports settings the web server cannot run without.
func Validate() error {
	if len(SessionSecret) < 32 {
		return errors.New("DASH_SESSION_SECRET must be at least 32 characters")
	}
	if (PasswordLogin == "") != (Password == "") {
		return errors.New("set both DASH_PASSWORD_LOGIN and DASH_PASSWORD, or neither")
	}
	if Password != "" && len(Password) < 12 {
		return errors.New("DASH_PASSWORD must be at least 12 characters")
	}
	return nil
}

// IsBootstrapOwner reports whether login is listed in DASH_BOOTSTRAP_OWNERS.
func IsBootstrapOwner(login string) bool { return slices.Contains(BootstrapOwners, login) }

// AppURL prefixes a site path with AppSubURL: AppURL("/admin") is "/finance/admin".
func AppURL(path string) string {
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") {
		return AppSubURL + path
	}
	return path
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
