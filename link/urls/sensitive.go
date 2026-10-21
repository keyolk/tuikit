package urls

import (
	"net/url"
	"strings"
)

// Ported from ccproxy's link guard (cmd/ccproxy/link_guard.go, CPLAT-12912):
// the monitor there shows captured request and response text, and a URL in
// it can carry a credential. Opening one hands the secret to the browser
// (history, sync, referrers), and an OSC 8 link puts it one cmd+click away.

// credentialNames are query or fragment keys that hold a secret by name.
// They are matched whole: "key" alone is a secret, "monkey" is not.
var credentialNames = map[string]bool{
	"key": true, "sig": true, "code": true, "auth": true, "authorization": true,
	"jwt": true, "bearer": true, "session": true, "sessionid": true, "sid": true,
	"pass": true, "pwd": true,
}

// credentialParts mark a key as a secret wherever they appear in it
// (access_token, X-Amz-Signature, client_secret, X-Amz-Credential, apiKey).
var credentialParts = []string{
	"token", "secret", "passw", "signature", "credential", "apikey", "api_key", "api-key",
}

// Sensitive reports whether u carries something that looks like a
// credential: userinfo (user:password@host), or a query or fragment key
// named like one (code, sig, session, …) or containing one (access_token,
// X-Amz-Signature, client_secret, apiKey). The fragment counts because the
// OAuth implicit flow returns #access_token=… there.
//
// It is a heuristic for what to hold back, not a guarantee, and it is
// separate from Valid on purpose: a review tool may still link a sensitive
// URL its own user produced, while a traffic monitor keeps it plain text.
// A string that does not parse as a URL is not Sensitive (and not Valid).
func Sensitive(u string) bool {
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	if p.User != nil {
		return true
	}
	return hasCredentialParam(p.RawQuery) || hasCredentialParam(p.Fragment)
}

func hasCredentialParam(raw string) bool {
	if raw == "" {
		return false
	}
	// Split by hand rather than url.ParseQuery: a malformed pair must not
	// hide the keys after it.
	for _, pair := range strings.FieldsFunc(raw, func(r rune) bool { return r == '&' || r == ';' }) {
		name, _, _ := strings.Cut(pair, "=")
		if dec, err := url.QueryUnescape(name); err == nil {
			name = dec
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if credentialNames[name] {
			return true
		}
		for _, part := range credentialParts {
			if strings.Contains(name, part) {
				return true
			}
		}
	}
	return false
}
