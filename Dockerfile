FROM golang:bullseye AS build

RUN apt update
RUN apt install -y make

RUN mkdir -p /build
WORKDIR /build

COPY . .

RUN make depend
RUN CGO_ENABLED=0 make

FROM debian:bullseye-slim

RUN apt update
RUN apt install -y avahi-autoipd ca-certificates

COPY --from=build /build/out/weron /usr/local/bin/weron

CMD ["/usr/local/bin/weron"]
