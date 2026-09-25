package orders

import (
	"errors"
	"testing"
	"time"
)

func TestNewTrade(t *testing.T) {
	executedAt := time.Date(
		2026,
		time.September,
		25,
		10,
		30,
		0,
		0,
		time.FixedZone("UTC-5", -5*60*60),
	)

	trade, err := NewTrade(
		" trade-001 ",
		" btcusd ",
		" buy-001 ",
		" sell-001 ",
		Price(6_000_025),
		Quantity(5_000_000),
		executedAt,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if trade.ID() != "trade-001" {
		t.Errorf("expected trade ID %q, got %q", "trade-001", trade.ID())
	}

	if trade.Symbol() != "BTCUSD" {
		t.Errorf("expected symbol %q, got %q", "BTCUSD", trade.Symbol())
	}

	if trade.BuyOrderID() != "buy-001" {
		t.Errorf(
			"expected buy order ID %q, got %q",
			"buy-001",
			trade.BuyOrderID(),
		)
	}

	if trade.SellOrderID() != "sell-001" {
		t.Errorf(
			"expected sell order ID %q, got %q",
			"sell-001",
			trade.SellOrderID(),
		)
	}

	if trade.Price() != Price(6_000_025) {
		t.Errorf(
			"expected price %d, got %d",
			6_000_025,
			trade.Price(),
		)
	}

	if trade.Quantity() != Quantity(5_000_000) {
		t.Errorf(
			"expected quantity %d, got %d",
			5_000_000,
			trade.Quantity(),
		)
	}

	expectedTime := executedAt.UTC()

	if !trade.ExecutedAt().Equal(expectedTime) {
		t.Errorf(
			"expected executedAt %v, got %v",
			expectedTime,
			trade.ExecutedAt(),
		)
	}

	if trade.ExecutedAt().Location() != time.UTC {
		t.Errorf(
			"expected UTC location, got %v",
			trade.ExecutedAt().Location(),
		)
	}
}

func TestNewTradeRejectsMissingID(t *testing.T) {
	_, err := NewTrade(
		"",
		"BTCUSD",
		"buy-001",
		"sell-001",
		Price(6_000_025),
		Quantity(5_000_000),
		time.Now(),
	)

	if !errors.Is(err, ErrTradeIDRequired) {
		t.Fatalf(
			"expected %v, got %v",
			ErrTradeIDRequired,
			err,
		)
	}
}

func TestNewTradeRejectsMissingSymbol(t *testing.T) {
	_, err := NewTrade(
		"trade-001",
		"",
		"buy-001",
		"sell-001",
		Price(6_000_025),
		Quantity(5_000_000),
		time.Now(),
	)

	if !errors.Is(err, ErrTradeSymbolRequired) {
		t.Fatalf(
			"expected %v, got %v",
			ErrTradeSymbolRequired,
			err,
		)
	}
}

func TestNewTradeRejectsMissingBuyOrderID(t *testing.T) {
	_, err := NewTrade(
		"trade-001",
		"BTCUSD",
		"",
		"sell-001",
		Price(6_000_025),
		Quantity(5_000_000),
		time.Now(),
	)

	if !errors.Is(err, ErrTradeBuyOrderIDRequired) {
		t.Fatalf(
			"expected %v, got %v",
			ErrTradeBuyOrderIDRequired,
			err,
		)
	}
}

func TestNewTradeRejectsMissingSellOrderID(t *testing.T) {
	_, err := NewTrade(
		"trade-001",
		"BTCUSD",
		"buy-001",
		"",
		Price(6_000_025),
		Quantity(5_000_000),
		time.Now(),
	)

	if !errors.Is(err, ErrTradeSellOrderIDRequired) {
		t.Fatalf(
			"expected %v, got %v",
			ErrTradeSellOrderIDRequired,
			err,
		)
	}
}

func TestNewTradeRejectsSameBuyAndSellOrder(t *testing.T) {
	_, err := NewTrade(
		"trade-001",
		"BTCUSD",
		"order-001",
		"order-001",
		Price(6_000_025),
		Quantity(5_000_000),
		time.Now(),
	)

	if !errors.Is(err, ErrTradeOrdersMustDiffer) {
		t.Fatalf(
			"expected %v, got %v",
			ErrTradeOrdersMustDiffer,
			err,
		)
	}
}

func TestNewTradeRejectsInvalidPrice(t *testing.T) {
	_, err := NewTrade(
		"trade-001",
		"BTCUSD",
		"buy-001",
		"sell-001",
		Price(0),
		Quantity(5_000_000),
		time.Now(),
	)

	if !errors.Is(err, ErrInvalidPrice) {
		t.Fatalf(
			"expected %v, got %v",
			ErrInvalidPrice,
			err,
		)
	}
}

func TestNewTradeRejectsInvalidQuantity(t *testing.T) {
	tests := []struct {
		name     string
		quantity Quantity
	}{
		{
			name:     "zero quantity",
			quantity: Quantity(0),
		},
		{
			name:     "negative quantity",
			quantity: Quantity(-1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTrade(
				"trade-001",
				"BTCUSD",
				"buy-001",
				"sell-001",
				Price(6_000_025),
				tt.quantity,
				time.Now(),
			)

			if !errors.Is(err, ErrTradeQuantityInvalid) {
				t.Fatalf(
					"expected %v, got %v",
					ErrTradeQuantityInvalid,
					err,
				)
			}
		})
	}
}

func TestNewTradeRejectsMissingExecutionTime(t *testing.T) {
	_, err := NewTrade(
		"trade-001",
		"BTCUSD",
		"buy-001",
		"sell-001",
		Price(6_000_025),
		Quantity(5_000_000),
		time.Time{},
	)

	if !errors.Is(err, ErrTradeExecutedAtRequired) {
		t.Fatalf(
			"expected %v, got %v",
			ErrTradeExecutedAtRequired,
			err,
		)
	}
}
