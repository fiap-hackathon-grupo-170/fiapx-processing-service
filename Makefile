.PHONY: build run test test-integration lint migrate-up migrate-down docker-build docker-run clean

APP_NAME=fiapx-processing-service
DOCKER_TAG=latest

build:
	go build -o bin/$(APP_NAME) ./cmd/worker

run: build
	./bin/$(APP_NAME)

test:
	go test ./... -short -count=1 -race

test-integration:
	go test ./tests/integration/... -count=1 -timeout=300s -v

lint:
	golangci-lint run ./...

migrate-up:
	migrate -database "$(DATABASE_URL)" -path migrations up

migrate-down:
	migrate -database "$(DATABASE_URL)" -path migrations down

docker-build:
	docker build -t $(APP_NAME):$(DOCKER_TAG) .

docker-run: docker-build
	docker run --rm --env-file .env --network fiapx-network $(APP_NAME):$(DOCKER_TAG)

clean:
	rm -rf bin/ /tmp/fiapx/
