FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN mkdir -p /app/secrets && go run ./cmd/genkey > /app/secrets/jwt_ed25519.pem
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/migrate /app/migrate
COPY --from=build /app/secrets /app/secrets
ENV JWT_PRIVATE_KEY_FILE=secrets/jwt_ed25519.pem
EXPOSE 8080
USER nonroot:nonroot

CMD ["/app/server"]
