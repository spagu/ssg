package deploy

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

// deployFTP uploads o.Dir over FTP. The target is ftp://[user@]host[:port]/base/path
// (from --deploy-target); the password comes from FTP_PASSWORD, the username from the
// URL or FTP_USERNAME (default "anonymous").
func deployFTP(ctx context.Context, o Options) (string, error) {
	u, err := parseDeployURL(o.Target, "ftp", 21)
	if err != nil {
		return "", err
	}
	user, pass := o.credentials(u, "FTP_USERNAME", "FTP_PASSWORD")
	if user == "" {
		user = "anonymous"
	}

	conn, err := ftp.Dial(u.Host, ftp.DialWithContext(ctx), ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return "", fmt.Errorf("connecting to %s: %w", u.Host, err)
	}
	defer func() { _ = conn.Quit() }()
	if err := conn.Login(user, pass); err != nil {
		return "", fmt.Errorf("ftp login: %w", err)
	}

	files, err := walkFiles(o.Dir)
	if err != nil {
		return "", err
	}
	base := strings.Trim(u.Path, "/")
	made := map[string]bool{}
	o.logf("☁️  Uploading %d files over FTP to %s…", len(files), u.Host)
	for _, f := range files {
		remote := path.Join(base, f.Rel)
		ftpMakeDirs(conn, path.Dir(remote), made)
		if err := conn.Stor(remote, bytes.NewReader(f.Data)); err != nil {
			return "", fmt.Errorf("storing %s: %w", remote, err)
		}
	}
	return "ftp://" + u.Host + "/" + base, nil
}

// ftpMakeDirs creates each ancestor directory of a remote path, remembering which
// ones already exist so repeated segments are not recreated.
func ftpMakeDirs(conn *ftp.ServerConn, dir string, made map[string]bool) {
	if dir == "" || dir == "." || dir == "/" || made[dir] {
		return
	}
	ftpMakeDirs(conn, path.Dir(dir), made)
	_ = conn.MakeDir(dir) // best-effort: already-exists is not fatal
	made[dir] = true
}
