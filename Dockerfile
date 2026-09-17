FROM golang:1.27-alpine AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=build /server /server

EXPOSE 8000

CMD ["/server"]