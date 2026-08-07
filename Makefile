.PHONY: fmt vet test build run install clean

fmt:
	gofmt -w .

vet:
	go vet ./...

test:
	go test ./...

build:
	go build -o openstack-mcp-server ./cmd/openstack-mcp-server

run:
	go run ./cmd/openstack-mcp-server

install: fmt build
	install -m 755 openstack-mcp-server $(HOME)/.local/bin/openstack-mcp-server

clean:
	rm -f openstack-mcp-server
