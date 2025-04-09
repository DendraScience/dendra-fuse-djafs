
main: *.go clean
	@echo "Building..."
	export GOARCH=amd64; \
	export CGO_ENABLED=0;\
	export GitCommit=`git rev-parse HEAD | cut -c -7`;\
	export GitTag=$$(TAG=`git tag --contains $$(git rev-parse HEAD) | sort -R | tr '\n' ' '`; if [ "$$(printf "$$TAG")" ]; then printf "$$TAG"; else printf "undefined"; fi);\
	go build -ldflags "-X main.GitCommit=$$GitCommit -X main.Tag=$$GitTag" -o djafs main.go
	@printf "\e[32mSuccess!\e[39m\n"
clean:  
	@printf "Cleaning up \e[32mmain\e[39m...\n"
	rm -f main djafs
	rm -f *.zip

install: clean main
	mv djafs "$$GOPATH/bin/$(PKG)"

vet: 
	@echo "Running go vet..."
	@go vet || (printf "\e[31mGo vet failed, exit code $$?\e[39m\n"; exit 1)
	@printf "\e[32mGo vet success!\e[39m\n"

