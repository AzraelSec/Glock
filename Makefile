.PHONY: test build install run compile

test:
	go test ./...

build:
	go build -o glock cmd/cli/main.go

install: build
	cp glock ~/.local/bin/glock

run:
	go run main.go

clean:
	rm glock

compile:
	echo "Compiling for every OS and Platform"
	GOOS=linux GOARCH=arm go build -o bin/glock-linux-arm cmd/cli/main.go
	GOOS=linux GOARCH=arm64 go build -o bin/main-linux-arm64 cmd/cli/main.go

