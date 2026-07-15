CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    category_id bigint NOT NULL,
    name text NOT NULL,
    description text,
    -- Money is stored in minor units: 1999 = $19.99. Never float.
    price_cents bigint NOT NULL,
    stock bigint DEFAULT 0 NOT NULL,
    sku text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT chk_products_price_cents CHECK (price_cents >= 0),
    CONSTRAINT chk_products_stock CHECK (stock >= 0),
    CONSTRAINT fk_categories_products FOREIGN KEY (category_id) REFERENCES categories(id)
);

-- Partial: a soft-deleted product must not reserve its SKU forever.
CREATE UNIQUE INDEX uniq_products_sku ON products USING btree (sku) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_category_id ON products USING btree (category_id);
CREATE INDEX idx_products_deleted_at ON products USING btree (deleted_at);
