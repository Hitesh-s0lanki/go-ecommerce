CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id bigint NOT NULL,
    status varchar(20) DEFAULT 'pending' NOT NULL,
    -- Money is stored in minor units. See products.price_cents.
    total_amount_cents bigint NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT chk_orders_total_amount_cents CHECK (total_amount_cents >= 0),
    CONSTRAINT fk_users_orders FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX idx_orders_user_id ON orders USING btree (user_id);
CREATE INDEX idx_orders_status ON orders USING btree (status);
CREATE INDEX idx_orders_deleted_at ON orders USING btree (deleted_at);
