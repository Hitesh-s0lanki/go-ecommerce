CREATE TABLE product_images (
    id BIGSERIAL PRIMARY KEY,
    product_id bigint NOT NULL,
    url text NOT NULL,
    alt_text text,
    is_primary boolean DEFAULT false NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT fk_products_images FOREIGN KEY (product_id) REFERENCES products(id)
);

CREATE INDEX idx_product_images_product_id ON product_images USING btree (product_id);
CREATE INDEX idx_product_images_deleted_at ON product_images USING btree (deleted_at);
