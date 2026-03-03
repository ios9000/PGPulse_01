.PHONY: build test lint docker-build docker-up docker-down clean

build:
	go build -o pgpulse-server ./cmd/pgpulse-server/
	go build -o pgpulse-agent ./cmd/pgpulse-agent/

test:
	go test -race ./cmd/... ./internal/...

lint:
	golangci-lint run

docker-build:
	docker build -f deploy/docker/Dockerfile -t pgpulse:dev .

docker-up:
	docker-compose -f deploy/docker/docker-compose.yml up -d

docker-down:
	docker-compose -f deploy/docker/docker-compose.yml down

clean:
	rm -f pgpulse-server pgpulse-agent
	rm -f pgpulse-server.exe pgpulse-agent.exe
