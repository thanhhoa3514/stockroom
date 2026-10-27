.PHONY: build run test docker-build docker-run docker-test
TEST_FLAGS ?=
build:
	go build -buildvcs=false -o bin/stockroom ./cmd/stockroom
run:
	go run -buildvcs=false ./cmd/stockroom $(ARGS)
test:
	@test -z "$$(gofmt -l .)"
	go vet ./...
	go test $(TEST_FLAGS) ./...
	@printf 'All checks passed.\n'

docker-build:
	docker compose build

docker-run:
	@mkdir -p data
	STOCKROOM_UID=$$(id -u) STOCKROOM_GID=$$(id -g) docker compose run --rm stockroom $(ARGS)

docker-test:
	docker build --target test -t stockroom-test .
	docker run --rm stockroom-test
