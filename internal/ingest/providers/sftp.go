package providers

import (
	"context"
	"fmt"
	"io"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPConfig exposes required connection parameters for SFTP hosts.
type SFTPConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string // Or use PrivateKey
	PrivateKey []byte
	BaseDir    string // Directory to scan
}

// SFTPProvider implements the ingest.Provider interface for SFTP servers.
type SFTPProvider struct {
	config Config
	sftp   *SFTPConfig
	client *sftp.Client
	conn   *ssh.Client
}

// NewSFTPProvider initializes a new SFTP ingest provider.
func NewSFTPProvider(cfg Config, sftpCfg SFTPConfig) (*SFTPProvider, error) {
	p := &SFTPProvider{
		config: cfg,
		sftp:   &sftpCfg,
	}

	if err := p.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to sftp: %w", err)
	}

	return p, nil
}

func (s *SFTPProvider) connect() error {
	var authModes []ssh.AuthMethod
	if s.sftp.Password != "" {
		authModes = append(authModes, ssh.Password(s.sftp.Password))
	}
	if len(s.sftp.PrivateKey) > 0 {
		signer, err := ssh.ParsePrivateKey(s.sftp.PrivateKey)
		if err != nil {
			return fmt.Errorf("invalid private key: %w", err)
		}
		authModes = append(authModes, ssh.PublicKeys(signer))
	}

	config := &ssh.ClientConfig{
		User: s.sftp.Username,
		Auth: authModes,
		// In production, you should use ssh.FixedHostKey(key) or a callback
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec G106 - requires user configuration in production mapping
	}

	addr := fmt.Sprintf("%s:%d", s.sftp.Host, s.sftp.Port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh dial failed: %w", err)
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("sftp client creation failed: %w", err)
	}

	s.conn = conn
	s.client = client
	return nil
}

// Type returns the provider identifier.
func (s *SFTPProvider) Type() string {
	return "sftp"
}

// ListFiles retrieves a list of available files to process based on the configured BaseDir.
func (s *SFTPProvider) ListFiles(ctx context.Context) ([]FileInfo, error) {
	// Simple reconnect logic if the connection dropped
	if _, err := s.client.Getwd(); err != nil {
		_ = s.Close()
		if err := s.connect(); err != nil {
			return nil, fmt.Errorf("failed to reconnect sftp: %w", err)
		}
	}

	entries, err := s.client.ReadDir(s.sftp.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("sftp read dir error: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories for flat polling
		}

		// SFTP paths should be rejoined correctly
		fullPath := s.sftp.BaseDir + "/" + entry.Name()

		files = append(files, FileInfo{
			Provider:   s.Type(),
			Path:       fullPath,
			Size:       entry.Size(),
			ModifiedAt: entry.ModTime(),
		})

		if len(files) >= s.config.MaxBatchSize && s.config.MaxBatchSize > 0 {
			break
		}
	}

	return files, nil
}

// DownloadFile streams a specific file from SFTP.
func (s *SFTPProvider) DownloadFile(ctx context.Context, filePath string) (io.ReadCloser, error) {
	// Reconnect if needed
	if _, err := s.client.Getwd(); err != nil {
		_ = s.Close()
		if err := s.connect(); err != nil {
			return nil, fmt.Errorf("failed to reconnect sftp: %w", err)
		}
	}

	// Open file on the remote server
	remoteFile, err := s.client.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sftp file: %w", err)
	}

	return remoteFile, nil
}

// Ack acknowledges that the file was successfully processed by deleting or moving it.
func (s *SFTPProvider) Ack(ctx context.Context, filePath string) error {
	if s.config.DeleteOnSuccess {
		return s.client.Remove(filePath)
	}

	if s.config.ArchivePath != "" {
		// Moving file to Archive
		// Ensure archive dir exists or just attempt rename
		dest := s.config.ArchivePath + "/" + extractFilename(filePath)
		return s.client.Rename(filePath, dest)
	}

	return nil
}

// Nack reports that the file failed processing.
func (s *SFTPProvider) Nack(ctx context.Context, filePath string) error {
	// Optional DLQ suffix or move
	dest := filePath + ".error"
	return s.client.Rename(filePath, dest)
}

// Close shuts down the persistent SSH/SFTP connection.
func (s *SFTPProvider) Close() error {
	var err1, err2 error
	if s.client != nil {
		err1 = s.client.Close()
	}
	if s.conn != nil {
		err2 = s.conn.Close()
	}

	if err1 != nil {
		return err1
	}
	return err2
}

// Helper to reliably extract the basename from an SFTP path.
func extractFilename(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
