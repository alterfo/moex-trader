.PHONY: test vet check

test:
	go test ./...

vet:
	go vet ./...

check: test vet
