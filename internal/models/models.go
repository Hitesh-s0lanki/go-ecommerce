package models

// All returns every model, in dependency order (parents before children).
//
// Migrations and tests both build the schema from this list, so a new model
// only needs registering here.
func All() []any {
	return []any{
		&User{},
		&RefreshToken{},
		&Category{},
		&Product{},
		&ProductImage{},
		&Cart{},
		&CartItem{},
		&Order{},
		&OrderItem{},
	}
}
