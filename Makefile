.PHONY: dev

up: 
	docker compose -f docker-compose-dev.yaml up -d
	docker compose -f docker-compose-dev.yaml down dev
	docker compose -f docker-compose-dev.yaml up dev

down:
	docker compose -f docker-compose-dev.yaml down

log:
	docker compose -f docker-compose-dev.yaml logs dev --follow

dev:
	docker compose -f docker-compose-dev.yaml up dev

dev-down:
	docker compose -f docker-compose-dev.yaml down dev

VERSION := $(shell cat VERSION)

build-binary:
	@rm -rf upload
	@CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -o upload/Darwin/x86_64/events-api cmd/main.go
	@CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o upload/Darwin/arm64/events-api cmd/main.go
	@CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o upload/Linux/x86_64/events-api cmd/main.go
	@CGO_ENABLED=0 GOOS=linux   GOARCH=386   go build -o upload/Linux/i386/events-api cmd/main.go
	@CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -o upload/Linux/arm64/events-api cmd/main.go

push-binary: build-binary
	@cp dev/scripts/download.sh upload/download.sh
	@echo 'latest version: $(VERSION)' > upload/metadata.txt
	@aws s3 cp --recursive upload/ s3://rwtools/events-api

REPO := risingwavelabs/events-api

build-docker:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/events-api-amd64 cmd/main.go
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/events-api-arm64 cmd/main.go
	@docker buildx build --sbom=true --platform linux/amd64,linux/arm64 -t $(REPO):$(VERSION) -t $(REPO):latest -f dev/Dockerfile --load .

push-docker: build-docker
	@docker push $(REPO):$(VERSION)
	@docker push $(REPO):latest

ci: push-docker push-binary
