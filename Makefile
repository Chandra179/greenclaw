.PHONY: build test clean docs docker-build docker-run

BIN := greenclaw

docs:
	swag init -g cmd/app/main.go -o cmd/app/docs

build: docs
	go build -o $(BIN) ./cmd/app

test:
	go test ./...

clean:
	rm -f $(BIN)

br:
	swag init -g cmd/app/main.go -o cmd/app/docs
	docker compose build
	docker compose up

r:
	docker compose up -d
	
g:
	nvidia-smi -l 1