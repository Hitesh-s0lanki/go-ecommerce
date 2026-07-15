package server

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// idParam reads a positive integer path parameter, answering 400 itself when
// it is missing, negative, zero, or not a number.
//
// Every :id route needs this, so it lives here once rather than as the same
// six-line ParseUint block copied into each handler.
func idParam(c *gin.Context, name string) (uint, bool) {
	// 32 bits, matching the uint the models use for ids: parsing as 64 would
	// let a large value wrap silently on conversion.
	id, err := strconv.ParseUint(c.Param(name), 10, 32)
	if err != nil {
		utils.BadRequestResponse(c, "invalid "+name, err)
		return 0, false
	}

	// Postgres identity columns start at 1, so 0 is never a real row, and
	// ParseUint accepts it.
	if id == 0 {
		utils.BadRequestResponse(c, "invalid "+name, nil)
		return 0, false
	}

	return uint(id), true
}
