CREATE TABLE cart_items (
    id BIGSERIAL PRIMARY KEY,
    cart_id bigint NOT NULL,
    product_id bigint NOT NULL,
    quantity bigint NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT chk_cart_items_quantity CHECK (quantity > 0),
    CONSTRAINT fk_carts_cart_items FOREIGN KEY (cart_id) REFERENCES carts(id),
    CONSTRAINT fk_products_cart_items FOREIGN KEY (product_id) REFERENCES products(id)
);

CREATE INDEX idx_cart_items_cart_id ON cart_items USING btree (cart_id);
CREATE INDEX idx_cart_items_product_id ON cart_items USING btree (product_id);
CREATE INDEX idx_cart_items_deleted_at ON cart_items USING btree (deleted_at);
