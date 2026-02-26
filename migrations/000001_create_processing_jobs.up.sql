CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE processing_jobs (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         VARCHAR(255)     NOT NULL,
    video_key       VARCHAR(1024)    NOT NULL,
    zip_key         VARCHAR(1024)    NOT NULL DEFAULT '',
    status          VARCHAR(20)      NOT NULL DEFAULT 'PENDING',
    frame_count     INTEGER          NOT NULL DEFAULT 0,
    file_size       BIGINT           NOT NULL DEFAULT 0,
    video_duration  DOUBLE PRECISION NOT NULL DEFAULT 0,
    attempt         INTEGER          NOT NULL DEFAULT 0,
    max_attempts    INTEGER          NOT NULL DEFAULT 7,
    error_message   TEXT             NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_processing_jobs_user_id ON processing_jobs(user_id);
CREATE INDEX idx_processing_jobs_status ON processing_jobs(status);
CREATE INDEX idx_processing_jobs_created_at ON processing_jobs(created_at);
