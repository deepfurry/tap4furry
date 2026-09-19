// Package mail implements the authentication consumer's delivery boundary.
// Challenge messages are delivered after database commit; raw tokens never queue.
package mail

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"uuid"
)

var errDelivery = errors.New("challenge mail unavailable")

// Disabled is explicit delivery failure for development and tests.
type Disabled struct{}

func (Disabled) SendEmailVerification(context.Context, string, string) error { return errDelivery }
func (Disabled) SendPasswordReset(context.Context, string, string) error     { return errDelivery }

type Local struct {
	directory *os.Root
	origin    string
}

// NewLocal confines captures to privateRoot even through nested symbolic links.
// Composition supplies the ignored repository .local root; tests use TempDir.
func NewLocal(privateRoot, directory, origin string) (*Local, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errDelivery
	}
	rootPath, err := filepath.Abs(privateRoot)
	if err != nil {
		return nil, errDelivery
	}
	dirPath, err := filepath.Abs(directory)
	if err != nil {
		return nil, errDelivery
	}
	rel, err := filepath.Rel(rootPath, dirPath)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return nil, errDelivery
	}
	if err = os.MkdirAll(rootPath, 0700); err != nil {
		return nil, errDelivery
	}
	info, err := os.Lstat(rootPath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errDelivery
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, errDelivery
	}
	defer root.Close()
	if err = root.MkdirAll(rel, 0700); err != nil {
		return nil, errDelivery
	}
	dir, err := root.OpenRoot(rel)
	if err != nil {
		return nil, errDelivery
	}
	info, err = dir.Stat(".")
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		dir.Close()
		return nil, errDelivery
	}
	return &Local{directory: dir, origin: origin}, nil
}
func (l *Local) Close() error { return l.directory.Close() }
func (l *Local) SendEmailVerification(ctx context.Context, email, token string) error {
	return l.send(ctx, email, token, "Verify your Tap4Furry email", "/verify-email")
}
func (l *Local) SendPasswordReset(ctx context.Context, email, token string) error {
	return l.send(ctx, email, token, "Reset your Tap4Furry password", "/reset-password")
}
func (l *Local) send(ctx context.Context, email, token, subject, path string) error {
	if ctx.Err() != nil || strings.ContainsAny(email, "\r\n") {
		return errDelivery
	}
	message := struct{ To, Subject, Link string }{email, subject, l.origin + path + "#token=" + url.QueryEscape(token)}
	data, err := json.Marshal(message)
	if err != nil {
		return errDelivery
	}
	name := uuid.NewV7().String() + ".json"
	file, err := l.directory.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errDelivery
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = l.directory.Remove(name)
		return errDelivery
	}
	return nil
}
