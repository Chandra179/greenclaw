.PHONY: build test run run-file run-json clean

BIN := temp

build:
	go build -o $(BIN) .

test:
	go test ./...

# Usage: make run URL=https://example.com
run: build
	./$(BIN) $(URL)

# Usage: make run-file FILE=urls.txt
run-file: build
	./$(BIN) --urls-file $(FILE)

# Usage: make run-json URL=https://example.com
run-json: build
	./$(BIN) --output json $(URL)

clean:
	rm -f $(BIN)
