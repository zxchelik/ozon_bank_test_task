-- +goose Up
CREATE TABLE links (
    id BIGSERIAL PRIMARY KEY,
    original_url TEXT NOT NULL,
    short_code VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ NULL,
    access_count BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT links_original_url_key UNIQUE (original_url),
    CONSTRAINT links_short_code_key UNIQUE (short_code),
    CONSTRAINT links_short_code_length_check CHECK (char_length(short_code) = 10),
    CONSTRAINT links_access_count_check CHECK (access_count >= 0)
);

-- +goose Down
DROP TABLE links;
