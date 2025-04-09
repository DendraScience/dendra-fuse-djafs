
main: *.go
	go build main.go
clean:  
	@printf "Cleaning up \e[32mmain\e[39m...\n"
	rm -f main dendra-archive-fuse
	rm -f *.zip

install: clean main
	mv dendra-archive-fuse "$$GOPATH/bin/$(PKG)"

vet: 
	@echo "Running go vet..."
	@go vet || (printf "\e[31mGo vet failed, exit code $$?\e[39m\n"; exit 1)
	@printf "\e[32mGo vet success!\e[39m\n"

