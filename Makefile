COMPOSE ?= docker compose

.PHONY: gate obu receiver calc aggregator proto test bootstrap bootstrap-build bootstrap-status bootstrap-logs bootstrap-down bootstrap-reset clean

gate:
	@go build -o bin/gate ./gateway
	@./bin/gate

obu:
	@go build -o bin/obu obu/main.go
	@./bin/obu

receiver:
	@go build -o bin/receiver ./data_receiver
	@./bin/receiver

calc:
	@go build -o bin/calc ./dist-calc
	@./bin/calc

aggregator:
	@go build -o bin/invoice ./aggregator
	@./bin/invoice

proto:
	@protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative types/ptypes.proto

test:
	@go test ./...
	@go test -race ./...
	@go vet ./...

bootstrap:
	@$(COMPOSE) up --build --detach --wait
	@$(COMPOSE) ps

bootstrap-build:
	@$(COMPOSE) build

bootstrap-status:
	@$(COMPOSE) ps

bootstrap-logs:
	@$(COMPOSE) logs --follow --tail=100

bootstrap-down:
	@$(COMPOSE) down --remove-orphans

bootstrap-reset:
	@$(COMPOSE) down --volumes --remove-orphans

clean:
	@rm -rf bin data
