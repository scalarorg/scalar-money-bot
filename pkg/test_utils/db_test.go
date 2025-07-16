package testutils_test

import (
	"context"
	"scalar-money-bot/internal/database"
	testutils "scalar-money-bot/pkg/test_utils"
	"testing"
)

func TestPrepareTestPostgresDB(t *testing.T) {
	testutils.RunWithTestDB(func(_ context.Context, _ *database.Repository) error {
		return nil
	})
}
