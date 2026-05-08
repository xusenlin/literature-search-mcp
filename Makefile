BINARY := literature-mcp

-include .env
export

.PHONY: build dev clean

build:
	go build -o $(BINARY) .

dev: build
	npx @modelcontextprotocol/inspector ./$(BINARY)

clean:
	rm -f $(BINARY)
