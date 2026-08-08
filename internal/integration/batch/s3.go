package batch

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Secrets struct {
	AccessKeyID     string
	SecretAccessKey string
}

type S3Provider struct {
	client *minio.Client
	policy S3Policy
}

func NewS3Provider(source SourceRevision, secrets S3Secrets) (*S3Provider, error) {
	if source.Validate() != nil || source.Provider != ProviderS3 || source.S3 == nil ||
		secrets.AccessKeyID == "" || secrets.SecretAccessKey == "" {
		return nil, ErrProviderUnavailable
	}
	client, err := minio.New(source.S3.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(secrets.AccessKeyID, secrets.SecretAccessKey, ""),
		Secure: source.S3.UseTLS, Region: source.S3.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: configure S3", ErrProviderUnavailable)
	}
	return &S3Provider{client: client, policy: *cloneS3(source.S3)}, nil
}

func (p *S3Provider) Type() ProviderType { return ProviderS3 }

func (p *S3Provider) List(ctx context.Context, limit int) ([]Object, error) {
	if p == nil || p.client == nil || ctx == nil || limit < 1 {
		return nil, ErrProviderUnavailable
	}
	versioning, err := p.client.GetBucketVersioning(ctx, p.policy.Bucket)
	if err != nil || !versioning.Enabled() {
		return nil, fmt.Errorf("%w: S3 bucket versioning is required", ErrProviderUnavailable)
	}
	objects := make([]Object, 0, limit)
	for info := range p.client.ListObjects(ctx, p.policy.Bucket, minio.ListObjectsOptions{
		Prefix: p.policy.InputPrefix + "/", Recursive: true, WithVersions: true,
	}) {
		if info.Err != nil {
			return nil, fmt.Errorf("%w: list S3 objects", ErrProviderUnavailable)
		}
		if strings.HasSuffix(info.Key, "/") || info.IsDeleteMarker {
			continue
		}
		object := Object{
			Provider: ProviderS3, Path: info.Key, Version: s3Version(info.VersionID),
			ETag: normalizeETag(info.ETag), Size: info.Size,
			RemoteModifiedAtAdvisory: info.LastModified.UTC(),
		}
		if object.validate() != nil || parseS3Version(object.Version) == "" {
			return nil, ErrInvalidObject
		}
		objects = append(objects, object)
		if len(objects) == limit {
			break
		}
	}
	return objects, nil
}

func (p *S3Provider) OpenAt(ctx context.Context, object Object, offset int64) (io.ReadCloser, error) {
	if p == nil || p.client == nil || ctx == nil || object.Provider != ProviderS3 ||
		object.validate() != nil || offset < 0 || offset > object.Size {
		return nil, ErrInvalidObject
	}
	if err := p.verifyObject(ctx, object); err != nil {
		return nil, err
	}
	options := minio.GetObjectOptions{VersionID: parseS3Version(object.Version)}
	if offset > 0 {
		if err := options.SetRange(offset, 0); err != nil {
			return nil, ErrInvalidObject
		}
	}
	return p.client.GetObject(ctx, p.policy.Bucket, object.Path, options)
}

func (p *S3Provider) Digest(ctx context.Context, object Object) (string, error) {
	reader, err := p.OpenAt(ctx, object, 0)
	if err != nil {
		return "", err
	}
	digest, digestErr := digestReader(reader)
	closeErr := reader.Close()
	if digestErr != nil {
		return "", fmt.Errorf("%w: hash S3 object", ErrProviderUnavailable)
	}
	if closeErr != nil {
		return "", fmt.Errorf("%w: close S3 object", ErrProviderUnavailable)
	}
	if err := p.verifyObject(ctx, object); err != nil {
		return "", err
	}
	return digest, nil
}

func (p *S3Provider) PrepareArchive(ctx context.Context, object Object, expectedDigest string) (string, error) {
	if !validSHA256Digest(expectedDigest) {
		return "", ErrArchiveCollision
	}
	if err := p.verifyObject(ctx, object); err != nil {
		return "", err
	}
	destination, err := p.archiveKey(object, expectedDigest)
	if err != nil {
		return "", err
	}
	if exists, err := p.objectExists(ctx, destination); err != nil {
		return "", err
	} else if exists {
		digest, err := p.digestKey(ctx, destination)
		if err != nil || digest != expectedDigest {
			return "", ErrArchiveCollision
		}
		return "s3://" + p.policy.Bucket + "/" + destination, nil
	}

	reader, err := p.OpenAt(ctx, object, 0)
	if err != nil {
		return "", err
	}
	options := minio.PutObjectOptions{ContentType: "application/hl7-v2+er7"}
	options.SetMatchETagExcept("*")
	_, putErr := p.client.PutObject(ctx, p.policy.Bucket, destination, reader, object.Size, options)
	closeErr := reader.Close()
	if putErr != nil || closeErr != nil {
		return "", fmt.Errorf("%w: copy S3 archive", ErrProviderUnavailable)
	}
	digest, err := p.digestKey(ctx, destination)
	if err != nil || digest != expectedDigest {
		_ = p.client.RemoveObject(ctx, p.policy.Bucket, destination, minio.RemoveObjectOptions{})
		return "", ErrArchiveCollision
	}
	if err := p.verifyObject(ctx, object); err != nil {
		return "", err
	}
	return "s3://" + p.policy.Bucket + "/" + destination, nil
}

func (p *S3Provider) DeleteSource(ctx context.Context, object Object, expectedDigest string) error {
	if !validSHA256Digest(expectedDigest) {
		return ErrInvalidObject
	}
	versionID := parseS3Version(object.Version)
	if versionID == "" {
		return ErrInvalidObject
	}
	info, err := p.client.StatObject(ctx, p.policy.Bucket, object.Path, minio.StatObjectOptions{VersionID: versionID})
	if err != nil {
		code := minio.ToErrorResponse(err).Code
		if code == "NoSuchKey" || code == "NoSuchObject" || code == "NotFound" {
			return nil
		}
		return fmt.Errorf("%w: stat S3 source before delete", ErrProviderUnavailable)
	}
	if info.VersionID != versionID || info.Size != object.Size || normalizeETag(info.ETag) != object.ETag {
		return ErrObjectChanged
	}
	if err := p.client.RemoveObject(ctx, p.policy.Bucket, object.Path, minio.RemoveObjectOptions{VersionID: versionID}); err != nil {
		return fmt.Errorf("%w: delete S3 source", ErrProviderUnavailable)
	}
	return nil
}

func (p *S3Provider) Close() error { return nil }

func (p *S3Provider) verifyObject(ctx context.Context, object Object) error {
	versionID := parseS3Version(object.Version)
	if versionID == "" {
		return ErrInvalidObject
	}
	info, err := p.client.StatObject(ctx, p.policy.Bucket, object.Path, minio.StatObjectOptions{VersionID: versionID})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" || minio.ToErrorResponse(err).Code == "NoSuchObject" {
			return fmt.Errorf("%w: source object missing", ErrObjectChanged)
		}
		return fmt.Errorf("%w: stat S3 object", ErrProviderUnavailable)
	}
	// Version ID plus entity tag is the exact-object identity re-verified before
	// every read, archive, and delete. Remote modification time takes no part.
	if info.VersionID != versionID || info.Size != object.Size || normalizeETag(info.ETag) != object.ETag {
		return ErrObjectChanged
	}
	return nil
}

func (p *S3Provider) archiveKey(object Object, digest string) (string, error) {
	relative := strings.TrimPrefix(object.Path, strings.TrimSuffix(p.policy.InputPrefix, "/")+"/")
	if !validRelativePrefix(relative) {
		return "", ErrInvalidObject
	}
	return path.Join(p.policy.ArchivePrefix, strings.TrimPrefix(digest, "sha256:"), relative), nil
}

func (p *S3Provider) objectExists(ctx context.Context, key string) (bool, error) {
	_, err := p.client.StatObject(ctx, p.policy.Bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	code := minio.ToErrorResponse(err).Code
	if code == "NoSuchKey" || code == "NoSuchObject" || code == "NotFound" {
		return false, nil
	}
	return false, fmt.Errorf("%w: stat S3 archive", ErrProviderUnavailable)
}

func (p *S3Provider) digestKey(ctx context.Context, key string) (string, error) {
	reader, err := p.client.GetObject(ctx, p.policy.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return "", err
	}
	digest, digestErr := digestReader(reader)
	closeErr := reader.Close()
	if digestErr != nil || closeErr != nil {
		return "", ErrProviderUnavailable
	}
	return digest, nil
}

// normalizeETag strips the optional quoting and weak-comparison prefix so the
// listing value and the head-object value compare exactly.
func normalizeETag(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "W/")
	return strings.Trim(value, `"`)
}

func s3Version(value string) string {
	if value == "" || value == "null" {
		return ""
	}
	return "version:" + value
}

func parseS3Version(value string) string {
	version := strings.TrimPrefix(value, "version:")
	if version == value || version == "" || version == "null" {
		return ""
	}
	return version
}
