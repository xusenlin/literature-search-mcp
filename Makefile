BINARY := literature-mcp-new

-include .env
export

.PHONY: build dev clean

build:
	go build -o $(BINARY) .

build-s:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY) .

dev: build
	npx @modelcontextprotocol/inspector ./$(BINARY)

clean:
	rm -f $(BINARY)
