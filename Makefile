.PHONY: run stop lint lint-fix tidy

run:
	docker compose --env-file .env.local up -d --build

stop:
	docker compose --env-file .env.local down

lint:
	docker compose exec app golangci-lint run

lint-fix:
	docker compose exec app golangci-lint run --fix

tidy:
	docker compose exec app go mod tidy