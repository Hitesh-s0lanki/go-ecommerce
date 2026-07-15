package utils_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func newContext() (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	return c, rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}

	return body
}

func TestSuccessResponse(t *testing.T) {
	c, rec := newContext()
	utils.SuccessResponse(c, "done", gin.H{"id": 1})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := decode(t, rec)
	if body["success"] != true {
		t.Error("success = false, want true")
	}
	// Absent fields must be omitted, not rendered as null/"".
	if _, ok := body["error"]; ok {
		t.Error("error key present on a success response, want omitted")
	}
}

func TestCreatedResponse(t *testing.T) {
	c, rec := newContext()
	utils.CreatedResponse(c, "created", gin.H{"id": 1})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

// A 4xx describes what the caller did wrong, so its detail is safe to return.
func TestClientErrorIncludesDetail(t *testing.T) {
	c, rec := newContext()
	utils.BadRequestResponse(c, "invalid payload", errors.New("field 'email' is required"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	body := decode(t, rec)
	if got := body["error"]; got != "field 'email' is required" {
		t.Errorf("error = %v, want the validation detail", got)
	}
}

// A 5xx is an internal failure: its text can carry schema details, file paths,
// or driver internals, so it must never reach the client.
func TestServerErrorHidesDetail(t *testing.T) {
	const secret = `pq: relation "users" does not exist`

	c, rec := newContext()
	utils.InternalServerErrorResponse(c, "something went wrong", errors.New(secret))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	if got := rec.Body.String(); got == "" {
		t.Fatal("empty body")
	}

	body := decode(t, rec)
	if _, ok := body["error"]; ok {
		t.Errorf("error key present on a 5xx response: %q — internal detail must not leak", body["error"])
	}
	if body["message"] != "something went wrong" {
		t.Errorf("message = %v, want the generic message", body["message"])
	}

	// The detail must still be recorded for the request log.
	if len(c.Errors) != 1 {
		t.Fatalf("c.Errors has %d entries, want 1 (detail must be logged)", len(c.Errors))
	}
	if got := c.Errors[0].Err.Error(); got != secret {
		t.Errorf("logged error = %q, want %q", got, secret)
	}
}

func TestUnauthorizedForbiddenNotFound(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*gin.Context, string)
		want int
	}{
		{"unauthorized", utils.UnauthorizedResponse, http.StatusUnauthorized},
		{"forbidden", utils.ForbiddenResponse, http.StatusForbidden},
		{"not found", utils.NotFoundResponse, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := newContext()
			tt.fn(c, "nope")

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}

			body := decode(t, rec)
			if body["success"] != false {
				t.Error("success = true, want false")
			}
		})
	}
}

func TestPaginatedSuccessResponse(t *testing.T) {
	c, rec := newContext()
	utils.PaginatedSuccessResponse(c, "listed", []int{1, 2}, utils.PaginationMeta{
		Page: 1, Limit: 2, Total: 10, TotalPages: 5,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := decode(t, rec)

	meta, ok := body["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta missing or wrong type: %v", body["meta"])
	}
	if meta["total_pages"] != float64(5) {
		t.Errorf("meta.total_pages = %v, want 5", meta["total_pages"])
	}
}
