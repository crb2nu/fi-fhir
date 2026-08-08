-- Slice 4.1b3 moves batch receipt provenance onto verifiable facts.
--
-- created_at is the server-owned custody timestamp: the moment this exact
-- object version was first durably admitted under this tenant, source, and
-- source revision. It is the authoritative received-at for every receipt and
-- canonical event derived from the object, and it is stable across lease
-- reclaim, worker restart, and checkpoint resume.
--
-- The remote modification time is renamed to state its trust level. It remains
-- available for operator diagnostics and as a change-detection input to the
-- SFTP synthetic version, and it participates in no trust or audit decision.
ALTER TABLE integration_batch_objects
    RENAME COLUMN object_modified_at TO remote_modified_at_advisory;

-- Exact provider-owned version identity, re-verified on every read.
ALTER TABLE integration_batch_objects
    ADD COLUMN object_version TEXT NOT NULL DEFAULT '';

-- S3 entity tag observed at listing and re-verified at every read, archive, and
-- delete. SFTP has no server-issued entity tag and stores the empty string.
ALTER TABLE integration_batch_objects
    ADD COLUMN object_etag TEXT NOT NULL DEFAULT '';

-- Marshaled SHA-256 continuation state for the streaming content digest. It
-- lets a resumed poll continue the same hash over the exact admitted bytes
-- instead of trusting a later re-read.
ALTER TABLE integration_batch_objects
    ADD COLUMN digest_state TEXT NOT NULL DEFAULT '';

-- NOT VALID so the constraint governs every row written from this revision
-- forward without inventing provenance for rows admitted before it existed.
-- Those rows keep the empty defaults and are visibly distinguishable.
ALTER TABLE integration_batch_objects
    ADD CONSTRAINT integration_batch_objects_provenance_chk CHECK (
        (provider = 's3' AND object_version <> '' AND object_etag <> '')
        OR
        (provider = 'sftp' AND object_version <> '' AND object_etag = '')
    ) NOT VALID;

COMMENT ON COLUMN integration_batch_objects.created_at IS
    'Server-owned custody timestamp; authoritative received-at for derived receipts.';
COMMENT ON COLUMN integration_batch_objects.remote_modified_at_advisory IS
    'Remote-controlled modification time; advisory only, never a trust input.';
COMMENT ON COLUMN integration_batch_objects.content_digest IS
    'SHA-256 over the exact bytes streamed during admission, cross-checked before archive.';
