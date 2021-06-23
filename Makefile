megalan: go.sum
	go build .
go.sum:
	go get megalan
install: megalan
	cp megalan /usr/sbin
clean:
	rm -f megalan go.sum
test: megalan
	sudo ./megalan DEBUG=3 NIC=Test1 PORT=0 HOST=172.20.1.1 RT=20 FILE=test.db
