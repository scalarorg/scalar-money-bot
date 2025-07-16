package testutils

import (
	"context"
	"fmt"
	"scalar-money-bot/internal/database"
	"time"

	"log"

	"github.com/testcontainers/testcontainers-go"
	pgc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type RunWithTestDBFunc func(ctx context.Context, repo *database.Repository) error

func RunWithTestDB(callback RunWithTestDBFunc) {
	ctx := context.Background()

	POSTGRES_DB := "liquidation_bot_test"
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

	testDb, err := database.Initialize(fmt.Sprintf("postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable", POSTGRES_USER, POSTGRES_PASSWORD, port.Port(), POSTGRES_DB))
	if err != nil {
		log.Fatal("Failed to initialize database")
	}

	err = callback(ctx, database.NewRepository(testDb))
	if err != nil {
		log.Fatal("Error in callback")
	}

	if pgContainer != nil {
		log.Println("Terminating postgres container")
		// err = pgContainer.Terminate(ctx)
		// if err != nil {
		// 	log.Println("Failed to terminate postgres container")
		// }
	}
}
