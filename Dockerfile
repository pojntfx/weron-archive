FROM golang:bullseye AS build

RUN apt update
RUN apt install -y make

RUN mkdir -p /build
WORKDIR /build

COPY . .

RUN make depend
RUN CGO_ENABLED=0 make

FROM debian:bullseye

RUN apt update
RUN apt install -y avahi-autoipd

COPY --from=build /build/out/weron /usr/local/bin/weron

CMD ["/usr/local/bin/weron"]
