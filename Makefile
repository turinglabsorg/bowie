agent-image:
	docker build -t bowie-agent:latest .

bowie:
	go build -o bowie ./cmd/bowie

test: test-agent test-go

test-agent:
	cd agent && python -m pytest tests/ -v

test-go:
	go test ./... -v

.PHONY: agent-image bowie test test-agent test-go
