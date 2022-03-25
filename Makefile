
build: id_rsa main.go proxy/handlers.go proxy/proxy.go proxy/server.go
	go build -o bin/main main.go

sshkey: id_rsa
	keygen -f id_rsa

check:
	staticcheck ./...

run:
	go run main.go

sshd:
	docker run --rm -p 3222:3222 --name sshd sshd

docker:
	docker build -t sshd .

ls:
	ssh localhost -p 4222 ls