package = github.com/w31r4/gokill

.PHONY: release

release:
	mkdir -p release
	go get -u
	GOOS=darwin GOARCH=amd64 go build -o release/gokill-darwin-amd64 $(package)
	GOOS=linux GOARCH=amd64 go build -o release/gokill-linux-amd64 $(package)
	GOOS=linux GOARCH=386 go build -o release/gokill-linux-386 $(package)
	GOOS=linux GOARCH=arm64 go build -o release/gokill-linux-arm64 $(package)
	GOARM=7 GOOS=linux GOARCH=arm go build -o release/gokill-linux-arm7 $(package)
	GOARM=6 GOOS=linux GOARCH=arm go build -o release/gokill-linux-arm6 $(package)
	GOARM=5 GOOS=linux GOARCH=arm go build -o release/gokill-linux-arm5 $(package)