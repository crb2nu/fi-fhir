package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/storage"
)

type fakeStorageProvider struct {
	listRecursiveFiles []storage.FileInfo
	listRecursiveErr   error

	statByPath map[string]*storage.FileInfo
	statErr    error

	openByPath map[string]io.ReadCloser
	openErr    error

	putErr   error
	putCalls []putCall

	existsByPath map[string]bool
	existsErr    error

	deleteErr   error
	deleteCalls []string
}

type putCall struct {
	path string
	size int64
	data []byte
}

func (f *fakeStorageProvider) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return true, nil
}

func (f *fakeStorageProvider) Delete(ctx context.Context, path string) error {
	f.deleteCalls = append(f.deleteCalls, path)
	return f.deleteErr
}

func (f *fakeStorageProvider) Exists(ctx context.Context, path string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	if f.existsByPath == nil {
		return false, nil
	}
	return f.existsByPath[path], nil
}

func (f *fakeStorageProvider) ListRecursive(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	return f.listRecursiveFiles, f.listRecursiveErr
}

func (f *fakeStorageProvider) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	if f.openByPath == nil {
		return nil, errors.New("no openByPath configured")
	}
	reader, ok := f.openByPath[path]
	if !ok {
		return nil, errors.New("path not found")
	}
	return reader, nil
}

func (f *fakeStorageProvider) Put(ctx context.Context, path string, r io.Reader, size int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.putCalls = append(f.putCalls, putCall{
		path: path,
		size: size,
		data: data,
	})
	return f.putErr
}

func (f *fakeStorageProvider) Stat(ctx context.Context, path string) (*storage.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	if f.statByPath == nil {
		return nil, errors.New("no statByPath configured")
	}
	info, ok := f.statByPath[path]
	if !ok {
		return nil, errors.New("path not found")
	}
	return info, nil
}

type errOnCloseReadCloser struct {
	io.Reader
	closeErr error
}

func (e *errOnCloseReadCloser) Close() error {
	return e.closeErr
}

func withStorageProviderFactory(t *testing.T, provider storageProvider) {
	t.Helper()
	previous := storageProviderFactory
	storageProviderFactory = func() (storageProvider, error) { return provider, nil }
	t.Cleanup(func() { storageProviderFactory = previous })
}

func TestStorage_List_Success(t *testing.T) {
	now := time.Date(2026, 1, 14, 12, 34, 0, 0, time.UTC)
	provider := &fakeStorageProvider{
		listRecursiveFiles: []storage.FileInfo{
			{Path: "bucket/prefix/a.txt", Size: 1234, LastModified: now},
			{Path: "bucket/prefix/b.txt", Size: 5678, LastModified: now.Add(-time.Hour)},
		},
	}
	withStorageProviderFactory(t, provider)

	stdout, _, err := runCLI(t, "storage", "ls", "s3://bucket/prefix/")
	assertNoError(t, err)
	assertContains(t, stdout, "PATH")
	assertContains(t, stdout, "bucket/prefix/a.txt")
	assertContains(t, stdout, "bucket/prefix/b.txt")
}

func TestStorage_List_NoFiles(t *testing.T) {
	provider := &fakeStorageProvider{listRecursiveFiles: nil}
	withStorageProviderFactory(t, provider)

	stdout, _, err := runCLI(t, "storage", "ls", "s3://bucket/prefix/")
	assertNoError(t, err)
	assertContains(t, stdout, "(no files)")
}

func TestStorage_Get_Success(t *testing.T) {
	data := []byte("hello from s3")
	s3Path := "s3://bucket/key.txt"
	localPath := filepath.Join(t.TempDir(), "key.txt")

	provider := &fakeStorageProvider{
		statByPath: map[string]*storage.FileInfo{
			s3Path: {Path: s3Path, Size: int64(len(data)), LastModified: time.Now()},
		},
		openByPath: map[string]io.ReadCloser{
			s3Path: io.NopCloser(bytes.NewReader(data)),
		},
	}
	withStorageProviderFactory(t, provider)

	stdout, _, err := runCLI(t, "storage", "get", s3Path, localPath)
	assertNoError(t, err)
	assertContains(t, stdout, "Downloading")
	assertContains(t, stdout, "Downloaded")

	written, readErr := os.ReadFile(localPath)
	assertNoError(t, readErr)
	if !bytes.Equal(written, data) {
		t.Fatalf("downloaded file mismatch: got %q, want %q", string(written), string(data))
	}
}

func TestStorage_Get_ReaderCloseError(t *testing.T) {
	data := []byte("hello")
	closeErr := errors.New("close failed")
	s3Path := "s3://bucket/key.txt"
	localPath := filepath.Join(t.TempDir(), "key.txt")

	provider := &fakeStorageProvider{
		statByPath: map[string]*storage.FileInfo{
			s3Path: {Path: s3Path, Size: int64(len(data)), LastModified: time.Now()},
		},
		openByPath: map[string]io.ReadCloser{
			s3Path: &errOnCloseReadCloser{Reader: bytes.NewReader(data), closeErr: closeErr},
		},
	}
	withStorageProviderFactory(t, provider)

	_, _, err := runCLI(t, "storage", "get", s3Path, localPath)
	assertError(t, err)
	assertErrorContains(t, err, "close failed")
}

func TestStorage_Put_Success(t *testing.T) {
	content := "upload me"
	localPath := createTempFile(t, t.TempDir(), "upload-*.txt", content)
	s3Path := "s3://bucket/key.txt"

	provider := &fakeStorageProvider{}
	withStorageProviderFactory(t, provider)

	stdout, _, err := runCLI(t, "storage", "put", localPath, s3Path)
	assertNoError(t, err)
	assertContains(t, stdout, "Upload complete")

	if len(provider.putCalls) != 1 {
		t.Fatalf("expected 1 Put call, got %d", len(provider.putCalls))
	}
	if provider.putCalls[0].path != s3Path {
		t.Fatalf("expected Put path %q, got %q", s3Path, provider.putCalls[0].path)
	}
	if string(provider.putCalls[0].data) != content {
		t.Fatalf("expected Put data %q, got %q", content, string(provider.putCalls[0].data))
	}
}

func TestStorage_Delete_NotExists(t *testing.T) {
	s3Path := "s3://bucket/key.txt"
	provider := &fakeStorageProvider{
		existsByPath: map[string]bool{s3Path: false},
	}
	withStorageProviderFactory(t, provider)

	stdout, _, err := runCLI(t, "storage", "rm", s3Path)
	assertNoError(t, err)
	assertContains(t, stdout, "Object does not exist")
	if len(provider.deleteCalls) != 0 {
		t.Fatalf("expected no Delete calls, got %d", len(provider.deleteCalls))
	}
}

func TestStorage_Delete_Exists(t *testing.T) {
	s3Path := "s3://bucket/key.txt"
	provider := &fakeStorageProvider{
		existsByPath: map[string]bool{s3Path: true},
	}
	withStorageProviderFactory(t, provider)

	stdout, _, err := runCLI(t, "storage", "rm", s3Path)
	assertNoError(t, err)
	assertContains(t, stdout, "Deleted:")
	if len(provider.deleteCalls) != 1 || provider.deleteCalls[0] != s3Path {
		t.Fatalf("expected Delete called for %q, got %#v", s3Path, provider.deleteCalls)
	}
}

func TestStorage_Stat_Success(t *testing.T) {
	s3Path := "s3://bucket/key.txt"
	info := &storage.FileInfo{
		Path:         s3Path,
		Size:         1234,
		LastModified: time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC),
		ETag:         "etag123",
		ContentType:  "text/plain",
	}
	provider := &fakeStorageProvider{
		statByPath: map[string]*storage.FileInfo{s3Path: info},
	}
	withStorageProviderFactory(t, provider)

	stdout, _, err := runCLI(t, "storage", "stat", s3Path)
	assertNoError(t, err)
	assertContains(t, stdout, "Path:")
	assertContains(t, stdout, "etag123")
	assertContains(t, stdout, "text/plain")
}
