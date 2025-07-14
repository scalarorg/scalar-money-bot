package testutils_test

import (
	"context"
	"scalar-money-bot/internal/models"
	testutils "scalar-money-bot/pkg/test_utils"
	"testing"
)

func TestPrepareTestPostgresDB(t *testing.T) {
	testutils.RunWithTestDB(func(_ context.Context, _ *models.Repository) error {
		return nil
	})
}
