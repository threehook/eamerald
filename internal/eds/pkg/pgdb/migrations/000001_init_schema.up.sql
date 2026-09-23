CREATE TABLE objects (
    object_type text NOT NULL,
    object_id text NOT NULL,
    data bytea NOT NULL,
    etag text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (object_type, object_id)
);

CREATE TABLE relations (
    object_type text NOT NULL,
    object_id text NOT NULL,
    relation text NOT NULL,
    subject_type text NOT NULL,
    subject_id text NOT NULL,
    subject_relation text NOT NULL DEFAULT '',
    data bytea NOT NULL,
    etag text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (object_type, object_id, relation, subject_type, subject_id, subject_relation)
);

CREATE INDEX relations_by_object ON relations (object_type, object_id, relation);
CREATE INDEX relations_by_subject ON relations (subject_type, subject_id, relation);

CREATE TABLE manifest (
    name text PRIMARY KEY DEFAULT 'default',
    metadata bytea NOT NULL,
    body bytea NOT NULL,
    model jsonb NOT NULL
);
