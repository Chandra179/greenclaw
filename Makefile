.PHONY: build test clean docker-build docker-run

BIN := greenclaw

build:
	go build -o $(BIN) .

test:
	go test ./...

clean:
	rm -f $(BIN)

docker-build:
	docker compose build

docker-run:
	docker compose up
