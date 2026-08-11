-- Tracks already delivered to Telegram. Telegram file IDs never expire, so a
-- hit here replaces the whole download pipeline with a single send.
CREATE TABLE IF NOT EXISTS tracks (
    video_id   TEXT PRIMARY KEY,
    title      TEXT    NOT NULL,
    artist     TEXT    NOT NULL,
    duration   INTEGER NOT NULL,
    file_id    TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS tracks_created_at_idx ON tracks (created_at);
