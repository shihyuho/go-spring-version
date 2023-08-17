FROM golang:1.19-bullseye as builder
RUN mkdir /app
WORKDIR /app
ADD . .
RUN make build

FROM paketobuildpacks/run:tiny-cnb
COPY --from=builder /app/build/spring-version /usr/bin
USER root
ENTRYPOINT ["spring-version"]
