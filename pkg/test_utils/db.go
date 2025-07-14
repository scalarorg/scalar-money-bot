package testutils

import (
	"context"
	"fmt"
	"scalar-money-bot/internal/models"
	"time"

	"log"

	"github.com/testcontainers/testcontainers-go"
	pgc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type RunWithTestDBFunc func(ctx context.Context, repo *models.Repository) error

func RunWithTestDB(callback RunWithTestDBFunc) {
	ctx := context.Background()

	POSTGRES_DB := "liquidation_bot"
	POSTGRES_USER := "user"
	POSTGRES_PASSWORD := "password"

	pgContainer, err := pgc.Run(ctx,
		"postgres:latest",
		pgc.WithDatabase(POSTGRES_DB),
		pgc.WithUsername(POSTGRES_USER),
		pgc.WithPassword(POSTGRES_PASSWORD),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		log.Fatal("Failed to start postgres container")
	}

	port, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		log.Fatal("Failed to get postgres port")
	}

	testDb, err := gorm.Open(postgres.Open(fmt.Sprintf("host=127.0.0.1 port=%s user=%s password=%s dbname=%s sslmode=disable", port.Port(), POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatal("Failed to connect to database")
	}

	err = callback(ctx, models.NewRepository(testDb))
	if err != nil {
		log.Fatal("Error in callback")
	}

	if pgContainer != nil {
		log.Println("Terminating postgres container")
		err = pgContainer.Terminate(ctx)
		if err != nil {
			log.Println("Failed to terminate postgres container")
		}
	}
}
