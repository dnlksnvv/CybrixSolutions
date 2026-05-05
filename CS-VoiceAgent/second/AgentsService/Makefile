.PHONY: test e2e verify run build

# Локальные unit/API тесты (без Docker, без реальной MongoDB)
test:
	go test ./... -count=1

# Полный pipeline в Docker: mongo → сервис → unit+E2E в контейнере e2e-test
# E2E_WORKSPACE_ID задаёт изолированный workspace (по умолчанию timestamp, см. verify-and-e2e.md)
e2e:
	E2E_WORKSPACE_ID=$${E2E_WORKSPACE_ID:-ws_e2e_$$(date +%s)}; \
	export E2E_WORKSPACE_ID; \
	docker compose -f docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from e2e-test; \
	EXIT=$$?; \
	docker compose -f docker-compose.test.yml down -v --remove-orphans; \
	exit $$EXIT

# Сначала быстрые локальные тесты, затем Docker E2E (двойная проверка unit при необходимости)
verify: test e2e

# Обычный локальный запуск (нужны .env и Mongo)
run:
	go run ./cmd/agent-service

# Только собрать production-образ (distroless)
build:
	docker build -t agents-service:local --target production .
