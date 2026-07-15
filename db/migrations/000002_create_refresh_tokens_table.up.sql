CREATE TABLE refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id bigint NOT NULL,
    token text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT fk_users_refresh_tokens FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX uniq_refresh_tokens_token ON refresh_tokens USING btree (token) WHERE deleted_at IS NULL;
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens USING btree (user_id);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens USING btree (expires_at);
CREATE INDEX idx_refresh_tokens_deleted_at ON refresh_tokens USING btree (deleted_at);
