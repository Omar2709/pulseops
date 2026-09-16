package orders

type OrderStatus string

const (
	OrderStatusPending         OrderStatus = "PENDING"
	OrderStatusOpen            OrderStatus = "OPEN"
	OrderStatusPartiallyFilled OrderStatus = "PARTIALLY_FILLED"
	OrderStatusFilled          OrderStatus = "FILLED"
	OrderStatusCancelled       OrderStatus = "CANCELLED"
	OrderStatusRejected        OrderStatus = "REJECTED"
)

func (s OrderStatus) IsTerminal() bool {
	switch s {
	case OrderStatusFilled, OrderStatusCancelled, OrderStatusRejected:
		return true
	default:
		return false
	}
}

func (s OrderStatus) CanTransitionTo(next OrderStatus) bool {
	switch s {
	case OrderStatusPending:
		return next == OrderStatusOpen ||
			next == OrderStatusRejected

	case OrderStatusOpen:
		return next == OrderStatusPartiallyFilled ||
			next == OrderStatusFilled ||
			next == OrderStatusCancelled

	case OrderStatusPartiallyFilled:
		return next == OrderStatusFilled ||
			next == OrderStatusCancelled

	default:
		return false
	}
}
