package batch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type SFTPSecrets struct {
	KnownHostsPath       string
	Password             string
	PrivateKey           []byte
	PrivateKeyPassphrase []byte
}

type SFTPProvider struct {
	policy   SFTPPolicy
	secrets  SFTPSecrets
	callback ssh.HostKeyCallback
	client   *sftp.Client
	conn     *ssh.Client
}

func NewSFTPProvider(source SourceRevision, secrets SFTPSecrets) (*SFTPProvider, error) {
	if source.Validate() != nil || source.Provider != ProviderSFTP || source.SFTP == nil {
		return nil, ErrProviderUnavailable
	}
	info, err := os.Stat(secrets.KnownHostsPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxSourceRevisionSize {
		return nil, fmt.Errorf("%w: invalid known_hosts file", ErrProviderUnavailable)
	}
	callback, err := knownhosts.New(secrets.KnownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("%w: parse known_hosts", ErrProviderUnavailable)
	}
	provider := &SFTPProvider{
		policy: *cloneSFTP(source.SFTP), secrets: cloneSFTPSecrets(secrets), callback: callback,
	}
	if err := provider.connect(); err != nil {
		return nil, err
	}
	return provider, nil
}

func (p *SFTPProvider) Type() ProviderType { return ProviderSFTP }

func (p *SFTPProvider) List(ctx context.Context, limit int) ([]Object, error) {
	if ctx == nil || limit < 1 || p.ensureConnected() != nil {
		return nil, ErrProviderUnavailable
	}
	entries, err := p.client.ReadDir(p.policy.InputDirectory)
	if err != nil {
		return nil, fmt.Errorf("%w: list SFTP directory", ErrProviderUnavailable)
	}
	objects := make([]Object, 0, limit)
	for _, entry := range entries {
		if entry.IsDir() || entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() {
			continue
		}
		objectPath := path.Join(p.policy.InputDirectory, entry.Name())
		if !strings.HasPrefix(objectPath, p.policy.InputDirectory+"/") {
			return nil, ErrInvalidObject
		}
		object := Object{
			Provider: ProviderSFTP, Path: objectPath, Version: sftpVersion(entry),
			Size: entry.Size(), RemoteModifiedAtAdvisory: entry.ModTime().UTC(),
		}
		if object.validate() != nil {
			return nil, ErrInvalidObject
		}
		objects = append(objects, object)
		if len(objects) == limit {
			break
		}
	}
	return objects, nil
}

func (p *SFTPProvider) OpenAt(ctx context.Context, object Object, offset int64) (io.ReadCloser, error) {
	if ctx == nil || object.Provider != ProviderSFTP || object.validate() != nil ||
		offset < 0 || offset > object.Size || p.ensureConnected() != nil {
		return nil, ErrInvalidObject
	}
	if err := p.verifyObject(object); err != nil {
		return nil, err
	}
	file, err := p.client.Open(object.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: open SFTP object", ErrProviderUnavailable)
	}
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("%w: seek SFTP object", ErrProviderUnavailable)
		}
	}
	return file, nil
}

func (p *SFTPProvider) Digest(ctx context.Context, object Object) (string, error) {
	reader, err := p.OpenAt(ctx, object, 0)
	if err != nil {
		return "", err
	}
	digest, digestErr := digestReader(reader)
	closeErr := reader.Close()
	if digestErr != nil || closeErr != nil {
		return "", fmt.Errorf("%w: hash SFTP object", ErrProviderUnavailable)
	}
	if err := p.verifyObject(object); err != nil {
		return "", err
	}
	return digest, nil
}

func (p *SFTPProvider) PrepareArchive(ctx context.Context, object Object, expectedDigest string) (string, error) {
	if !validSHA256Digest(expectedDigest) || p.ensureConnected() != nil {
		return "", ErrArchiveCollision
	}
	if err := p.verifyObject(object); err != nil {
		return "", err
	}
	destination, err := p.archivePath(object, expectedDigest)
	if err != nil {
		return "", err
	}
	if info, err := p.client.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", ErrArchiveCollision
		}
		digest, digestErr := p.digestPath(destination)
		if digestErr != nil || digest != expectedDigest {
			return "", ErrArchiveCollision
		}
		return "sftp://" + net.JoinHostPort(p.policy.Host, fmt.Sprint(p.policy.Port)) + destination, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: stat SFTP archive", ErrProviderUnavailable)
	}
	if err := p.client.MkdirAll(path.Dir(destination)); err != nil {
		return "", fmt.Errorf("%w: create SFTP archive directory", ErrProviderUnavailable)
	}
	source, err := p.OpenAt(ctx, object, 0)
	if err != nil {
		return "", err
	}
	destinationFile, err := p.client.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		_ = source.Close()
		if info, statErr := p.client.Lstat(destination); statErr == nil &&
			info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			digest, digestErr := p.digestPath(destination)
			if digestErr == nil && digest == expectedDigest {
				return "sftp://" + net.JoinHostPort(p.policy.Host, fmt.Sprint(p.policy.Port)) + destination, nil
			}
		}
		return "", ErrArchiveCollision
	}
	_, copyErr := io.Copy(destinationFile, source)
	destinationCloseErr := destinationFile.Close()
	sourceCloseErr := source.Close()
	if copyErr != nil || destinationCloseErr != nil || sourceCloseErr != nil {
		_ = p.client.Remove(destination)
		return "", fmt.Errorf("%w: copy SFTP archive", ErrProviderUnavailable)
	}
	digest, err := p.digestPath(destination)
	if err != nil || digest != expectedDigest {
		_ = p.client.Remove(destination)
		return "", ErrArchiveCollision
	}
	if err := p.verifyObject(object); err != nil {
		return "", err
	}
	return "sftp://" + net.JoinHostPort(p.policy.Host, fmt.Sprint(p.policy.Port)) + destination, nil
}

func (p *SFTPProvider) DeleteSource(ctx context.Context, object Object, expectedDigest string) error {
	if p.ensureConnected() != nil {
		return ErrProviderUnavailable
	}
	if !validSHA256Digest(expectedDigest) {
		return ErrInvalidObject
	}
	if err := p.verifyObject(object); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	digest, err := p.Digest(ctx, object)
	if err != nil {
		return err
	}
	if digest != expectedDigest {
		return ErrObjectChanged
	}
	if err := p.client.Remove(object.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: delete SFTP source", ErrProviderUnavailable)
	}
	return nil
}

func (p *SFTPProvider) Close() error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.client != nil {
		errs = append(errs, p.client.Close())
		p.client = nil
	}
	if p.conn != nil {
		errs = append(errs, p.conn.Close())
		p.conn = nil
	}
	return errors.Join(errs...)
}

func (p *SFTPProvider) connect() error {
	auth, err := p.authMethods()
	if err != nil {
		return err
	}
	address := net.JoinHostPort(p.policy.Host, fmt.Sprint(p.policy.Port))
	networkConnection, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return fmt.Errorf("%w: dial SFTP", ErrProviderUnavailable)
	}
	connection, channels, requests, err := ssh.NewClientConn(networkConnection, address, &ssh.ClientConfig{
		User: p.policy.Username, Auth: auth, HostKeyCallback: p.callback, Timeout: 10 * time.Second,
	})
	if err != nil {
		_ = networkConnection.Close()
		return fmt.Errorf("%w: verify SFTP connection", ErrProviderUnavailable)
	}
	sshClient := ssh.NewClient(connection, channels, requests)
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return fmt.Errorf("%w: create SFTP client", ErrProviderUnavailable)
	}
	p.conn = sshClient
	p.client = sftpClient
	return nil
}

func (p *SFTPProvider) authMethods() ([]ssh.AuthMethod, error) {
	if p.policy.PasswordBinding != "" {
		if p.secrets.Password == "" {
			return nil, ErrProviderUnavailable
		}
		return []ssh.AuthMethod{ssh.Password(p.secrets.Password)}, nil
	}
	if len(p.secrets.PrivateKey) == 0 {
		return nil, ErrProviderUnavailable
	}
	var signer ssh.Signer
	var err error
	if len(p.secrets.PrivateKeyPassphrase) != 0 {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(p.secrets.PrivateKey, p.secrets.PrivateKeyPassphrase)
	} else {
		signer, err = ssh.ParsePrivateKey(p.secrets.PrivateKey)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: parse SFTP private key", ErrProviderUnavailable)
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
}

func (p *SFTPProvider) ensureConnected() error {
	if p == nil || p.callback == nil {
		return ErrProviderUnavailable
	}
	if p.client != nil {
		if _, err := p.client.Getwd(); err == nil {
			return nil
		}
	}
	_ = p.Close()
	return p.connect()
}

func (p *SFTPProvider) verifyObject(object Object) error {
	info, err := p.client.Lstat(object.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %w", ErrObjectChanged, os.ErrNotExist)
		}
		return fmt.Errorf("%w: stat SFTP object", ErrProviderUnavailable)
	}
	// The synthetic version folds size, modification time, and permissions into
	// a change-detection value. It is deliberately not provenance: the remote
	// side controls modification time, so content trust comes from the digest
	// computed over the exact bytes streamed during admission.
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		sftpVersion(info) != object.Version || info.Size() != object.Size ||
		!info.ModTime().UTC().Equal(object.RemoteModifiedAtAdvisory.UTC()) {
		return ErrObjectChanged
	}
	return nil
}

func (p *SFTPProvider) archivePath(object Object, digest string) (string, error) {
	relative := strings.TrimPrefix(object.Path, strings.TrimSuffix(p.policy.InputDirectory, "/")+"/")
	if !validRelativePrefix(relative) {
		return "", ErrInvalidObject
	}
	return path.Join(p.policy.ArchiveDirectory, strings.TrimPrefix(digest, "sha256:"), relative), nil
}

func (p *SFTPProvider) digestPath(value string) (string, error) {
	info, err := p.client.Lstat(value)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", ErrArchiveCollision
	}
	file, err := p.client.Open(value)
	if err != nil {
		return "", err
	}
	digest, digestErr := digestReader(file)
	closeErr := file.Close()
	if digestErr != nil || closeErr != nil {
		return "", ErrProviderUnavailable
	}
	return digest, nil
}

func sftpVersion(info os.FileInfo) string {
	hasher := sha256.New()
	_, _ = fmt.Fprintf(hasher, "%d\x00%d\x00%d", info.Size(), info.ModTime().UTC().UnixNano(), info.Mode().Perm())
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}

func cloneSFTPSecrets(secrets SFTPSecrets) SFTPSecrets {
	secrets.PrivateKey = append([]byte(nil), secrets.PrivateKey...)
	secrets.PrivateKeyPassphrase = append([]byte(nil), secrets.PrivateKeyPassphrase...)
	return secrets
}
