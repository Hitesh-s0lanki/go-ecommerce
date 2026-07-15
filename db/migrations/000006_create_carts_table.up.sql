CREATE TABLE carts (
    id BIGSERIAL PRIMARY KEY,
    user_id bigint NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT fk_users_cart FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Partial: one live cart per user, but a soft-deleted cart must not block a new one.
CREATE UNIQUE INDEX uniq_carts_user_id ON carts USING btree (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_carts_deleted_at ON carts USING btree (deleted_at);
