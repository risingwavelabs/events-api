.PHONY: dev

up: 
	docker compose -f docker-compose-dev.yaml up -d

down:
	docker compose -f docker-compose-dev.yaml down

log:
	docker compose -f docker-compose-dev.yaml logs dev --follow

dev:
	docker compose -f docker-compose-dev.yaml up dev

dev-down:
	docker compose -f docker-compose-dev.yaml down dev
