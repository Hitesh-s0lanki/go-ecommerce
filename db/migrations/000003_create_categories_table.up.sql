CREATE TABLE categories (
    id BIGSERIAL PRIMARY KEY,
    name text NOT NULL,
    description text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz
);

CREATE INDEX idx_categories_deleted_at ON categories USING btree (deleted_at);
