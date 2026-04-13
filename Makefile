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

invoicer:
	@go build -o bin/invoice ./aggregator
	@./bin/invoice

clean:
	@rm -rf bin data