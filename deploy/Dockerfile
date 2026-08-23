FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY server /app/server
COPY configs /app/configs
COPY migrations /app/migrations

USER 65532:65532
EXPOSE 3000

ENTRYPOINT ["/app/server"]
