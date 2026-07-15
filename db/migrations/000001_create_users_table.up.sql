CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email text NOT NULL,
    password text NOT NULL,
    first_name text NOT NULL,
    last_name text NOT NULL,
    phone text,
    is_active boolean DEFAULT true NOT NULL,
    role varchar(20) DEFAULT 'customer' NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz
);

-- Partial: a soft-deleted user must not reserve their email forever.
CREATE UNIQUE INDEX uniq_users_email ON users USING btree (email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_deleted_at ON users USING btree (deleted_at);
