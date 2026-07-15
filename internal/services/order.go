package services

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
)

// Order errors.
var (
	ErrOrderNotFound = errors.New("order not found")
	// ErrCartEmpty is returned when there is nothing to order.
	ErrCartEmpty = errors.New("cart is empty")
)

// OrderService turns carts into orders.
type OrderService struct {
	db *gorm.DB
}

// NewOrderService builds an OrderService.
func NewOrderService(db *gorm.DB) *OrderService {
	return &OrderService{db: db}
}

// Create places an order for everything in the user's cart.
//
// The whole thing is one transaction: stock comes down, the order and its lines
// are written, and the cart is emptied, all together or not at all. A partial
// checkout would either charge for stock nobody reserved or hold stock for an
// order that does not exist.
func (s *OrderService) Create(ctx context.Context, userID uint) (*dto.OrderResponse, error) {
	var resp *dto.OrderResponse

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cart, err := loadCartForCheckout(tx, userID)
		if err != nil {
			return err
		}

		if len(cart.CartItems) == 0 {
			return ErrCartEmpty
		}

		products, err := lockProductsForCheckout(tx, cart.CartItems)
		if err != nil {
			return err
		}

		order := models.Order{
			UserID: userID,
			Status: models.OrderStatusPending,
		}

		for i := range cart.CartItems {
			item := &cart.CartItems[i]

			product, ok := products[item.ProductID]
			if !ok {
				// Soft-deleted since it went in the cart.
				return fmt.Errorf("%w: product %d", ErrProductNotFound, item.ProductID)
			}

			if !product.IsActive {
				return fmt.Errorf("%w: %s", ErrProductUnavailable, product.Name)
			}

			if product.Stock < item.Quantity {
				return fmt.Errorf("%w for %s: %d in stock, %d ordered",
					ErrInsufficientStock, product.Name, product.Stock, item.Quantity)
			}

			// Safe because the row is locked: the read above cannot have gone
			// stale. The reference does this with an unlocked read and a Save
			// of the whole row, so two checkouts both read stock 5, both write
			// 3, and two units are sold that never existed.
			if err := tx.Model(&models.Product{}).
				Where("id = ?", product.ID).
				UpdateColumn("stock", gorm.Expr("stock - ?", item.Quantity)).Error; err != nil {
				return fmt.Errorf("decrement stock: %w", err)
			}

			order.OrderItems = append(order.OrderItems, models.OrderItem{
				ProductID: item.ProductID,
				Quantity:  item.Quantity,
				// Copied from the locked row, so the order records what the
				// customer actually pays and a later price change cannot
				// rewrite it.
				UnitPriceCents: product.PriceCents,
			})

			order.TotalAmountCents += product.PriceCents * int64(item.Quantity)
		}

		// Creates the order and its lines together, via the association.
		//
		// The reference has this inside the item loop, so a three-item cart
		// writes three orders — the first with one line, the next with two,
		// the last with three — and each with a running total.
		if err := tx.Create(&order).Error; err != nil {
			return fmt.Errorf("create order: %w", err)
		}

		// Hard delete: a cart line is working state, not history. The order is
		// the record, and soft-deleted lines would only sit there forever
		// holding a unique index against the products they name.
		if err := tx.Unscoped().Where("cart_id = ?", cart.ID).Delete(&models.CartItem{}).Error; err != nil {
			return fmt.Errorf("clear cart: %w", err)
		}

		loaded, err := loadOrder(tx, order.ID)
		if err != nil {
			return err
		}

		r := dto.NewOrderResponse(loaded)
		resp = &r

		return nil
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// List returns a page of the user's orders, newest first.
func (s *OrderService) List(ctx context.Context, userID uint, query dto.ListQuery) ([]dto.OrderResponse, dto.PageMeta, error) {
	query.Normalize()

	var total int64

	// Not ignored, unlike the reference's: a dropped count reports an empty
	// order history to a customer who has one.
	if err := s.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, dto.PageMeta{}, fmt.Errorf("count orders: %w", err)
	}

	meta := dto.NewPageMeta(query, total)

	if total == 0 {
		return []dto.OrderResponse{}, meta, nil
	}

	var orders []models.Order

	// id breaks ties on created_at: two orders placed in the same instant
	// would otherwise page in any order, repeating one and hiding another.
	if err := s.db.WithContext(ctx).
		Preload("OrderItems", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_items.id")
		}).
		Preload("OrderItems.Product").
		Preload("OrderItems.Product.Category").
		Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").
		Offset(query.Offset()).
		Limit(query.Limit).
		Find(&orders).Error; err != nil {
		return nil, dto.PageMeta{}, fmt.Errorf("list orders: %w", err)
	}

	resp := make([]dto.OrderResponse, len(orders))
	for i := range orders {
		resp[i] = dto.NewOrderResponse(&orders[i])
	}

	return resp, meta, nil
}

// Get returns one of the user's orders.
//
// Scoped to the caller: an order id belonging to someone else must read as
// missing. Answering 403 would confirm the order exists, and answering it at
// all would hand over another customer's purchase history.
func (s *OrderService) Get(ctx context.Context, userID, orderID uint) (*dto.OrderResponse, error) {
	var order models.Order

	err := s.db.WithContext(ctx).
		Preload("OrderItems", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_items.id")
		}).
		Preload("OrderItems.Product").
		Preload("OrderItems.Product.Category").
		Where("id = ? AND user_id = ?", orderID, userID).
		First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}

		return nil, fmt.Errorf("find order: %w", err)
	}

	resp := dto.NewOrderResponse(&order)

	return &resp, nil
}

// loadCartForCheckout reads the user's cart and its lines.
func loadCartForCheckout(tx *gorm.DB, userID uint) (*models.Cart, error) {
	var cart models.Cart

	err := tx.Preload("CartItems", func(db *gorm.DB) *gorm.DB {
		return db.Order("cart_items.id")
	}).Where("user_id = ?", userID).First(&cart).Error
	if err != nil {
		// No cart is the same thing as an empty one, and reads better than
		// the reference's "cart not found" on a customer's first checkout.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCartEmpty
		}

		return nil, fmt.Errorf("load cart: %w", err)
	}

	return &cart, nil
}

// lockProductsForCheckout takes a row lock on every product in the cart and
// returns them by id.
//
// SELECT ... FOR UPDATE is what makes the stock check mean anything: without
// it, another checkout can sell the last unit between this one's check and its
// write. The lock is held until the transaction ends, so the decrement below is
// safe.
//
// Ordered by id, and that ordering is the point: two checkouts sharing two
// products would otherwise be free to lock them in opposite orders and
// deadlock. Every transaction taking the same order cannot.
func lockProductsForCheckout(tx *gorm.DB, items []models.CartItem) (map[uint]*models.Product, error) {
	ids := make([]uint, 0, len(items))
	for i := range items {
		ids = append(ids, items[i].ProductID)
	}

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var products []models.Product

	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ?", ids).
		Order("id").
		Find(&products).Error; err != nil {
		return nil, fmt.Errorf("lock products: %w", err)
	}

	byID := make(map[uint]*models.Product, len(products))
	for i := range products {
		byID[products[i].ID] = &products[i]
	}

	return byID, nil
}

// loadOrder reads an order with everything needed to render it.
func loadOrder(tx *gorm.DB, orderID uint) (*models.Order, error) {
	var order models.Order

	err := tx.Preload("OrderItems", func(db *gorm.DB) *gorm.DB {
		return db.Order("order_items.id")
	}).
		Preload("OrderItems.Product").
		Preload("OrderItems.Product.Category").
		First(&order, orderID).Error
	if err != nil {
		return nil, fmt.Errorf("load order: %w", err)
	}

	return &order, nil
}
