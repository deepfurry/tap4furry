package resource

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/deepfurry/tap4furry/server/internal/taxonomy"
)

// NormalizeURL is conservative and performs no network I/O. In particular it
// keeps escaped paths, dot segments, query order/encoding and an empty query.
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !utf8.ValidString(raw) || u.Opaque != "" || u.User != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ErrValidation
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", ErrValidation
		}
		if (u.Scheme == "http" && n == 80) || (u.Scheme == "https" && n == 443) {
			port = ""
		}
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host = net.JoinHostPort(strings.Trim(host, "[]"), port)
	}
	u.Host, u.Fragment, u.RawFragment = host, "", ""
	value := u.String()
	if utf8.RuneCountInString(value) > 2048 {
		return "", ErrValidation
	}
	return value, nil
}

type Source struct {
	URL          string
	Label        *string
	Type         SourceType
	Availability SourceAvailabilityState
	Rights       SourceRightsStatus
	Primary      bool
}

func (s Source) Normalize() (Source, error) {
	if !s.Type.Valid() || !s.Availability.Valid() || !s.Rights.Valid() {
		return Source{}, ErrValidation
	}
	value, err := NormalizeURL(s.URL)
	if err != nil {
		return Source{}, err
	}
	label, err := taxonomy.OptionalText(s.Label, 80)
	if err != nil {
		return Source{}, ErrValidation
	}
	s.URL, s.Label = value, label
	return s, nil
}
