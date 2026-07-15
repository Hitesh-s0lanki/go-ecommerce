-- One line per product per cart.
--
-- Without this, adding the same product twice races: two concurrent requests
-- both find no existing row and both insert one, leaving a cart with two lines
-- for one product. The index is what lets the insert be a single atomic
-- ON CONFLICT ... DO UPDATE instead of a read followed by a write.
--
-- Partial, matching uniq_carts_user_id and uniq_products_sku: a soft-deleted
-- line must not reserve its product forever.
CREATE UNIQUE INDEX uniq_cart_items_cart_id_product_id
    ON cart_items USING btree (cart_id, product_id)
    WHERE deleted_at IS NULL;
