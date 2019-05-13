all:
	@go run policygen.go
install:
	@go run policygen.go | gofmt > policy.txt
	@mv policy.txt ../../rpc_policy.go
