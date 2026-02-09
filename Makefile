agent-image:
	docker build -t openbowie-agent:latest .

openbowie:
	go build -o openbowie ./cmd/openbowie

test: test-agent test-go

test-agent:
	cd agent && python -m pytest tests/ -v

test-go:
	go test ./... -v

.PHONY: agent-image openbowie test test-agent test-go
