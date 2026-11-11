# Build stage
FROM golang:1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /nautix .

# Runtime stage: the static binary only (the frontend is embedded in it)
FROM scratch
COPY --from=build /nautix /nautix
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/nautix"]
