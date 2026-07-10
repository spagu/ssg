package deploy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// deploySFTP uploads o.Dir over SSH/SFTP. The target is sftp://user@host[:port]/path
// (from --deploy-target). Authentication uses SSH_PASSWORD, or a private key from
// SSH_KEY_FILE (default ~/.ssh/id_rsa) with optional SSH_KEY_PASSPHRASE. Host keys are
// verified against ~/.ssh/known_hosts (no blind trust).
func deploySFTP(ctx context.Context, o Options) (string, error) {
	u, err := parseDeployURL(o.Target, "sftp", 22)
	if err != nil {
		return "", err
	}
	user, pass := o.credentials(u, "SSH_USERNAME", "SSH_PASSWORD")
	if user == "" {
		return "", fmt.Errorf("sftp needs a username in --deploy-target or SSH_USERNAME")
	}
	auth, err := sshAuthMethods(o, pass)
	if err != nil {
		return "", err
	}
	hostKeyCb, err := knownHostsCallback(o)
	if err != nil {
		return "", err
	}

	client, err := ssh.Dial("tcp", u.Host, &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: hostKeyCb,
		Timeout:         30 * time.Second,
	})
	if err != nil {
		return "", fmt.Errorf("ssh dial %s: %w", u.Host, err)
	}
	defer func() { _ = client.Close() }()
	_ = ctx // ssh.Dial has its own timeout; ctx reserved for symmetry

	sc, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("opening sftp: %w", err)
	}
	defer func() { _ = sc.Close() }()

	files, err := walkFiles(o.Dir)
	if err != nil {
		return "", err
	}
	base := strings.TrimRight(u.Path, "/")
	o.logf("☁️  Uploading %d files over SFTP to %s…", len(files), u.Host)
	for _, f := range files {
		remote := path.Join(base, f.Rel)
		if err := sftpWriteFile(sc, remote, f.Data); err != nil {
			return "", err
		}
	}
	return "sftp://" + u.Host + base, nil
}

// sftpWriteFile creates the remote file (and its parent directories) and writes data.
func sftpWriteFile(sc *sftp.Client, remote string, data []byte) error {
	if dir := path.Dir(remote); dir != "" && dir != "." {
		if err := sc.MkdirAll(dir); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	dst, err := sc.Create(remote)
	if err != nil {
		return fmt.Errorf("create %s: %w", remote, err)
	}
	defer func() { _ = dst.Close() }()
	if _, err := io.Copy(dst, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("write %s: %w", remote, err)
	}
	return nil
}

// sshAuthMethods builds the SSH auth chain: password when provided, otherwise a
// private key from SSH_KEY_FILE (default ~/.ssh/id_rsa).
func sshAuthMethods(o Options, pass string) ([]ssh.AuthMethod, error) {
	if pass != "" {
		return []ssh.AuthMethod{ssh.Password(pass)}, nil
	}
	keyPath := o.env("SSH_KEY_FILE")
	if keyPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("no SSH_PASSWORD and cannot locate home dir for a default key: %w", err)
		}
		keyPath = filepath.Join(home, ".ssh", "id_rsa")
	}
	pem, err := os.ReadFile(keyPath) // #nosec G304 -- key path is operator-supplied deploy credential
	if err != nil {
		return nil, fmt.Errorf("reading SSH key %s: %w", keyPath, err)
	}
	var signer ssh.Signer
	if pp := o.env("SSH_KEY_PASSPHRASE"); pp != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(pem, []byte(pp))
	} else {
		signer, err = ssh.ParsePrivateKey(pem)
	}
	if err != nil {
		return nil, fmt.Errorf("parsing SSH key: %w", err)
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
}

// knownHostsCallback verifies the server against ~/.ssh/known_hosts (or the file named
// by SSH_KNOWN_HOSTS), refusing unknown hosts rather than trusting blindly.
func knownHostsCallback(o Options) (ssh.HostKeyCallback, error) {
	khPath := o.env("SSH_KNOWN_HOSTS")
	if khPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot locate ~/.ssh/known_hosts: %w", err)
		}
		khPath = filepath.Join(home, ".ssh", "known_hosts")
	}
	cb, err := knownhosts.New(khPath)
	if err != nil {
		return nil, fmt.Errorf("loading known_hosts (%s): %w — add the host with ssh-keyscan first", khPath, err)
	}
	return cb, nil
}
