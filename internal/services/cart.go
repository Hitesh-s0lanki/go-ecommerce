package services

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
)

// Cart errors.
var (
	ErrCartItemNotFound = errors.New("cart item not found")
	// ErrInsufficientStock is returned when a cart would hold more of a
	// product than exists.
	ErrInsufficientStock = errors.New("insufficient stock")
	// ErrProductUnavailable is returned for a product that is not on sale.
	ErrProductUnavailable = errors.New("product is not available")
)

// CartService manages users' shopping carts.
type CartService struct {
	db *gorm.DB
}

// NewCartService builds a CartService.
func NewCartService(db *gorm.DB) *CartService {
	return &CartService{db: db}
}

// Get returns the user's cart, creating an empty one if they have none.
//
// The reference returns "record not found" when a user has never had a cart,
// so a fresh account's first GET /cart is a 404. An empty cart is not an
// error — it is what every new customer has.
func (s *CartService) Get(ctx context.Context, userID uint) (*dto.CartResponse, error) {
	var resp *dto.CartResponse

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cart, err := getOrCreateCart(tx, userID)
		if err != nil {
			return err
		}

		loaded, err := loadCart(tx, cart.ID)
		if err != nil {
			return err
		}

		r := dto.NewCartResponse(loaded)
		resp = &r

		return nil
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// AddItem puts a product in the cart, or adds to the quantity already there.
func (s *CartService) AddItem(ctx context.Context, userID uint, req *dto.AddToCartRequest) (*dto.CartResponse, error) {
	var resp *dto.CartResponse

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		product, err := loadPurchasableProduct(tx, req.ProductID)
		if err != nil {
			return err
		}

		cart, err := getOrCreateCart(tx, userID)
		if err != nil {
			return err
		}

		item := models.CartItem{
			CartID:    cart.ID,
			ProductID: req.ProductID,
			Quantity:  req.Quantity,
		}

		// One statement: insert the line, or add to the quantity already
		// there. The reference reads the row and then writes it, so two
		// concurrent adds both see nothing and insert, leaving two lines for
		// one product — and it discards the error from both writes.
		//
		// RETURNING gives back the resulting quantity, so the stock check
		// below tests the total rather than what this request happened to add.
		err = tx.Clauses(
			clause.OnConflict{
				Columns:     []clause.Column{{Name: "cart_id"}, {Name: "product_id"}},
				TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("deleted_at IS NULL")}},
				DoUpdates: clause.Assignments(map[string]any{
					"quantity":   gorm.Expr("cart_items.quantity + excluded.quantity"),
					"updated_at": gorm.Expr("NOW()"),
				}),
			},
			clause.Returning{},
		).Create(&item).Error
		if err != nil {
			return fmt.Errorf("add cart item: %w", err)
		}

		// A cart is not a reservation — stock is only truly held at checkout —
		// but letting someone stack 500 of a 3-stock item just to be refused
		// at the till is a worse answer than saying so now.
		if item.Quantity > product.Stock {
			return fmt.Errorf("%w: %d in stock, cart would hold %d",
				ErrInsufficientStock, product.Stock, item.Quantity)
		}

		loaded, err := loadCart(tx, cart.ID)
		if err != nil {
			return err
		}

		r := dto.NewCartResponse(loaded)
		resp = &r

		return nil
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// UpdateItem sets the quantity of a line the user owns.
func (s *CartService) UpdateItem(ctx context.Context, userID, itemID uint, req *dto.UpdateCartItemRequest) (*dto.CartResponse, error) {
	var resp *dto.CartResponse

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item models.CartItem

		// Scoped to the user's own cart in the query itself: a line id from
		// another customer's cart must read as missing, not as forbidden,
		// which would confirm it exists.
		err := tx.Joins("JOIN carts ON carts.id = cart_items.cart_id AND carts.deleted_at IS NULL").
			Where("cart_items.id = ? AND carts.user_id = ?", itemID, userID).
			First(&item).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCartItemNotFound
			}

			return fmt.Errorf("find cart item: %w", err)
		}

		product, err := loadPurchasableProduct(tx, item.ProductID)
		if err != nil {
			return err
		}

		if req.Quantity > product.Stock {
			return fmt.Errorf("%w: %d in stock, %d requested",
				ErrInsufficientStock, product.Stock, req.Quantity)
		}

		if err := tx.Model(&item).Update("quantity", req.Quantity).Error; err != nil {
			return fmt.Errorf("update cart item: %w", err)
		}

		loaded, err := loadCart(tx, item.CartID)
		if err != nil {
			return err
		}

		r := dto.NewCartResponse(loaded)
		resp = &r

		return nil
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// RemoveItem drops a line from the user's cart.
func (s *CartService) RemoveItem(ctx context.Context, userID, itemID uint) error {
	// The subquery is what confines the delete to this user's cart. Without
	// it, any customer could delete any line by guessing its id.
	cartIDs := s.db.Model(&models.Cart{}).Select("id").Where("user_id = ?", userID)

	result := s.db.WithContext(ctx).
		Where("id = ? AND cart_id IN (?)", itemID, cartIDs).
		Delete(&models.CartItem{})

	if result.Error != nil {
		return fmt.Errorf("remove cart item: %w", result.Error)
	}

	// Delete reports no error for a line that was never there, which the
	// reference returns as a 200.
	if result.RowsAffected == 0 {
		return ErrCartItemNotFound
	}

	return nil
}

// getOrCreateCart returns the user's cart, creating it if this is their first.
//
// Registration already makes one, so the create is for accounts that predate
// it, or whose cart was removed.
func getOrCreateCart(tx *gorm.DB, userID uint) (*models.Cart, error) {
	var cart models.Cart

	err := tx.Where("user_id = ?", userID).First(&cart).Error
	if err == nil {
		return &cart, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("find cart: %w", err)
	}

	cart = models.Cart{UserID: userID}

	// DO NOTHING rather than an error: two requests from the same new user can
	// both find no cart and both insert, and the partial unique index makes
	// one of them lose. Losing that race is not a failure — the cart exists,
	// which is all the caller wanted.
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cart)
	if result.Error != nil {
		// The user is gone but their token has not expired.
		if errors.Is(result.Error, gorm.ErrForeignKeyViolated) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("create cart: %w", result.Error)
	}

	// DoNothing means no row came back, so the winner's cart is read instead.
	if result.RowsAffected == 0 {
		if err := tx.Where("user_id = ?", userID).First(&cart).Error; err != nil {
			return nil, fmt.Errorf("find cart after conflict: %w", err)
		}
	}

	return &cart, nil
}

// loadCart reads a cart with the products needed to price it.
func loadCart(tx *gorm.DB, cartID uint) (*models.Cart, error) {
	var cart models.Cart

	// The category comes too: a cart line renders the product, and
	// ProductResponse carries its category.
	err := tx.Preload("CartItems", func(db *gorm.DB) *gorm.DB {
		// Stable order, so a cart does not reshuffle between requests.
		return db.Order("cart_items.id")
	}).
		Preload("CartItems.Product").
		Preload("CartItems.Product.Category").
		Preload("CartItems.Product.Images", primaryImageFirst).
		First(&cart, cartID).Error
	if err != nil {
		return nil, fmt.Errorf("load cart: %w", err)
	}

	return &cart, nil
}

// loadPurchasableProduct reads a product that may be added to a cart.
//
// A soft-deleted product reads as not found, and an inactive one as
// unavailable: the reference checks neither, so a withdrawn product can still
// be added to a cart and bought.
func loadPurchasableProduct(tx *gorm.DB, productID uint) (*models.Product, error) {
	var product models.Product

	if err := tx.First(&product, productID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProductNotFound
		}

		return nil, fmt.Errorf("find product: %w", err)
	}

	if !product.IsActive {
		return nil, fmt.Errorf("%w: %s", ErrProductUnavailable, product.Name)
	}

	return &product, nil
}
