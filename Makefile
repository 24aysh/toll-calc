.PHONY: obu clean invoicer

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

clean:
	@rm -rf bin data