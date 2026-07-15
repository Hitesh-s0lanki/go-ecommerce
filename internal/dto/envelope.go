package dto

// The types below exist for the generated API documentation.
//
// Every endpoint answers with utils.Response, whose Data field is `any` —
// which tells a docs generator nothing about the payload. These spell out the
// concrete shape per endpoint. They are never constructed at runtime; the
// handlers still return utils.Response.

// AuthEnvelope is the response of the register, login, and refresh endpoints.
type AuthEnvelope struct {
	Success bool         `json:"success"  example:"true"`
	Message string       `json:"message"  example:"logged in"`
	Data    AuthResponse `json:"data"`
}

// UserEnvelope is the response of endpoints returning a single user.
type UserEnvelope struct {
	Success bool         `json:"success" example:"true"`
	Message string       `json:"message" example:"ok"`
	Data    UserResponse `json:"data"`
}

// CategoryEnvelope is the response of endpoints returning a single category.
type CategoryEnvelope struct {
	Success bool             `json:"success" example:"true"`
	Message string           `json:"message" example:"ok"`
	Data    CategoryResponse `json:"data"`
}

// CategoryListEnvelope is the response of the category list endpoint.
type CategoryListEnvelope struct {
	Success bool               `json:"success" example:"true"`
	Message string             `json:"message" example:"ok"`
	Data    []CategoryResponse `json:"data"`
}

// ProductEnvelope is the response of endpoints returning a single product.
type ProductEnvelope struct {
	Success bool            `json:"success" example:"true"`
	Message string          `json:"message" example:"ok"`
	Data    ProductResponse `json:"data"`
}

// ProductListEnvelope is the response of the product list endpoint, which
// carries page metadata alongside the rows.
type ProductListEnvelope struct {
	Success bool              `json:"success" example:"true"`
	Message string            `json:"message" example:"ok"`
	Data    []ProductResponse `json:"data"`
	Meta    PageMeta          `json:"meta"`
}

// MessageEnvelope is the response of endpoints returning no payload.
type MessageEnvelope struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"logged out"`
}

// ErrorEnvelope is the response of a failed request.
//
// The error field carries detail only for 4xx: a 5xx would otherwise leak
// schema names, file paths, or driver internals to the caller.
type ErrorEnvelope struct {
	Success bool   `json:"success"         example:"false"`
	Message string `json:"message"         example:"invalid credentials"`
	Error   string `json:"error,omitempty" example:"email is not available"`
}
