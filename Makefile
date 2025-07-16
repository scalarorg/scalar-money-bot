.PHONY: build run test clean docker-build docker-run docker-stop help

# Application name
APP_NAME := scalar-money-bot
DOCKER_IMAGE := $(APP_NAME):latest

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
BINARY_NAME := bin/$(APP_NAME)
BINARY_PATH := ./cmd/bot

# Development targets
build: ## Build the application
	$(GOBUILD) -o $(BINARY_NAME) -v $(BINARY_PATH)

run: ## Run the application
	$(GOBUILD) -o $(BINARY_NAME) -v $(BINARY_PATH)
	./$(BINARY_NAME)

test: ## Run tests
	$(GOTEST) -v ./...

test-coverage: ## Run tests with coverage
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

# Dependencies
deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) tidy

deps-update: ## Update dependencies
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

# Docker targets
docker-build: ## Build Docker image
	docker build -t $(DOCKER_IMAGE) .

docker: ## Run application with Docker Compose
	docker-compose up -d

docker-stop: ## Stop Docker Compose services
	docker-compose down

docker-logs: ## Show Docker Compose logs
	docker-compose logs -f

docker-clean: ## Clean Docker images and containers
	docker-compose down --volumes --remove-orphans

# Database targets
db-migrate: ## Run database migrations
	@echo "Running database migrations..."
	@if [ -f .env ]; then \
		set -a && source .env && set +a && \
		docker-compose exec postgres psql -U "${POSTGRES_USER:-user}" -d "${POSTGRES_DB:-liquidation_bot}" -f /docker-entrypoint-initdb.d/001_initial.sql; \
	else \
		echo "No .env file found. Please create one based on .env.example"; \
		exit 1; \
	fi

db-reset: ## Reset database
	docker-compose exec postgres psql -U user -d liquidation_bot -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"

# Environment setup
setup: ## Initial project setup
	cp .env.example .env
	@echo "Please edit .env file with your configuration"
	$(MAKE) deps

# Development helpers
dev: ## Run in development mode with auto-reload
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	air

lint: ## Run linter
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run

fmt: ## Format code
	$(GOCMD) fmt ./...

# Production targets
build-prod: ## Build for production
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -a -installsuffix cgo -ldflags '-extldflags "-static"' -o $(BINARY_NAME) $(BINARY_PATH)

deploy: ## Deploy to production (customize as needed)
	@echo "Deploying to production..."
	$(MAKE) docker-build
	# Add your deployment commands here


.PHONY: anvil setup-contracts
anvil:
	@if lsof -ti:8545 > /dev/null 2>&1; then \
		echo "Anvil already running on port 8545"; \
	else \
		anvil --port 8545 --allow-origin "*"; \
	fi
setup-contracts:
	@contract-deployer --config ./deploy.toml
