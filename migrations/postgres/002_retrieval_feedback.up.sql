CREATE TABLE retrieval_feedback (
    id              VARCHAR(26) PRIMARY KEY,
    target          VARCHAR(20)  NOT NULL CHECK (target IN ('memory', 'context')),
    feedback_type   VARCHAR(20)  NOT NULL CHECK (feedback_type IN ('helpful', 'irrelevant')),
    query           TEXT         NOT NULL DEFAULT '',
    tenant_id       VARCHAR(100),
    project_id      VARCHAR(100) NOT NULL,
    repo_id         VARCHAR(100),
    agent_id        VARCHAR(100),
    session_id      VARCHAR(100),
    environment     VARCHAR(50),
    result_ids      VARCHAR(26)[] NOT NULL DEFAULT '{}',
    selected_ids    VARCHAR(26)[] NOT NULL DEFAULT '{}',
    actor           VARCHAR(255) NOT NULL,
    metadata        JSONB        DEFAULT '{}',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_feedback_project    ON retrieval_feedback(project_id);
CREATE INDEX idx_feedback_target     ON retrieval_feedback(target, feedback_type);
CREATE INDEX idx_feedback_actor      ON retrieval_feedback(actor);
CREATE INDEX idx_feedback_created    ON retrieval_feedback(created_at DESC);
