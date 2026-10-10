# make        build 1337cli and 1337tui for this machine
# make dist   build Linux/Windows binaries + deploy files into dist/1337cli.tar.gz

BIN := 1337cli
LDFLAGS := -s -w

.PHONY: build test dist clean

build:
	go build -o $(BIN) .
	go build -o 1337tui ./tui

test:
	go test ./...

dist: test
	rm -rf dist && mkdir -p dist/1337cli/bin
	@for t in linux/amd64 linux/arm64 linux/arm windows/amd64; do \
		os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		for p in 1337cli:. 1337tui:./tui; do \
			name=$${p%%:*}; pkg=$${p#*:}; \
			echo "build $$name-$$os-$$arch$$ext"; \
			GOOS=$$os GOARCH=$$arch GOARM=7 CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' \
				-o dist/1337cli/bin/$$name-$$os-$$arch$$ext $$pkg || exit 1; \
		done; \
	done
	cp deploy/docker-compose.yml deploy/config.example deploy/install.sh dist/1337cli/
	COPYFILE_DISABLE=1 tar --no-xattrs -C dist -czf dist/1337cli.tar.gz 1337cli
	@echo "built dist/1337cli.tar.gz"

clean:
	rm -rf dist $(BIN) 1337tui
