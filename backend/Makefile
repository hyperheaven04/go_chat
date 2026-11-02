.PHONY: env up down logs test test-integration lint

DB_PORT ?= 55432
TEST_DATABASE_URL ?= postgres://chat:$(shell grep -s POSTGRES_PASSWORD .env | cut -d= -f2)@localhost:$(DB_PORT)/chat_test?sslmode=disable

env: ## generate .env with random secrets
	@test -f .env && echo ".env already exists" || { \
		echo "POSTGRES_PASSWORD=$$(openssl rand -hex 16)" > .env; \
		echo "JWT_SECRET=$$(openssl rand -base64 48)" >> .env; \
		echo "ALLOWED_ORIGINS=" >> .env; \
		echo "created .env"; }

up: env
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f server

test: ## unit tests, no external dependencies
	go test -race ./...

test-integration: env ## end-to-end tests against a separate chat_test database in docker compose
	docker compose up -d db --wait
	@docker compose exec -T db psql -U chat -d chat -tAc "SELECT 1 FROM pg_database WHERE datname = 'chat_test'" | grep -q 1 \
		|| docker compose exec -T db createdb -U chat chat_test
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -count=1 ./...

lint:
	go vet ./...
