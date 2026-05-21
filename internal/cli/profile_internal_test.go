package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestTodayCostForReturnsQueryUsageErrors(t *testing.T) {
	want := errors.New("query failed")
	_, err := todayCostFor(context.Background(), &Deps{
		Store:   todayCostStore{err: want},
		Pricing: todayCostPricing{},
	}, "work")
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestTodayCostForReturnsPricingErrors(t *testing.T) {
	want := errors.New("pricing failed")
	_, err := todayCostFor(context.Background(), &Deps{
		Store: todayCostStore{rows: []contracts.UsageRow{{
			Model: "model-a",
			Day:   time.Now().UTC(),
			Usage: contracts.Usage{InputTokens: 1},
		}}},
		Pricing: todayCostPricing{err: want},
	}, "work")
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

type todayCostStore struct {
	contracts.Store
	rows []contracts.UsageRow
	err  error
}

func (s todayCostStore) QueryUsage(_ context.Context, _ contracts.UsageQuery) ([]contracts.UsageRow, error) {
	return s.rows, s.err
}

type todayCostPricing struct {
	err error
}

func (p todayCostPricing) Cost(_ string, _ time.Time, _ contracts.Usage) (float64, error) {
	return 0, p.err
}

func (p todayCostPricing) LastUpdated() time.Time {
	return time.Time{}
}
