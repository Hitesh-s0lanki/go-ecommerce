CREATE TABLE order_items (
    id BIGSERIAL PRIMARY KEY,
    order_id bigint NOT NULL,
    product_id bigint NOT NULL,
    quantity bigint NOT NULL,
    -- Snapshot of the price at purchase, so later price changes do not
    -- rewrite order history.
    unit_price_cents bigint NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT chk_order_items_quantity CHECK (quantity > 0),
    CONSTRAINT chk_order_items_unit_price_cents CHECK (unit_price_cents >= 0),
    CONSTRAINT fk_orders_order_items FOREIGN KEY (order_id) REFERENCES orders(id),
    CONSTRAINT fk_products_order_items FOREIGN KEY (product_id) REFERENCES products(id)
);

CREATE INDEX idx_order_items_order_id ON order_items USING btree (order_id);
CREATE INDEX idx_order_items_product_id ON order_items USING btree (product_id);
CREATE INDEX idx_order_items_deleted_at ON order_items USING btree (deleted_at);
