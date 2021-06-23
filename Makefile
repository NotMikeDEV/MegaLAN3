megalan:
	go build .
install: megalan
	cp megalan /usr/sbin
clean:
	rm -f megalan
test: megalan
	sudo ./megalan DEBUG=3 NIC=Test1 PORT=0 HOST=172.20.1.1 RT=20
