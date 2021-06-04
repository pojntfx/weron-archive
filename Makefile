all: build

build:
	go build -o out/weron main.go

release:
	CGO_ENABLED=1 go build -ldflags="-extldflags=-static" -tags netgo -o out/release/weron.linux-$$(uname -m) main.go

install: release
	sudo install out/release/weron.linux-$$(uname -m) /usr/local/bin/weron
	sudo setcap cap_net_admin+ep /usr/local/bin/weron
	
dev:
	while [ -z "$$PID" ] || [ -n "$$(inotifywait -q -r -e modify pkg cmd main.go)" ]; do\
		$(MAKE);\
		kill -9 $$PID 2>/dev/null 1>&2;\
		wait $$PID;\
		sudo setcap cap_net_admin+ep out/weron;\
		out/weron & export PID="$$!";\
	done

clean:
	rm -rf out
	rm -rf ~/.local/share/weron

depend:
	echo 0