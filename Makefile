.PHONY: build test vet clean
build:
	mkdir -p build/licenses
	CGO_ENABLED=0 go build -tags mygo_noinspector -trimpath -ldflags='-s -w' -o build/ape-player ./cmd/ape-player
	cp internal/icons/lucide/LICENSE build/licenses/lucide-LICENSE

test:
	CGO_ENABLED=0 go test ./...

vet:
	CGO_ENABLED=0 go vet ./...
	CGO_ENABLED=0 go run github.com/egoist/mygo/cmd/mygo vet .
