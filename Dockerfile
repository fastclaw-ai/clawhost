FROM node:20-alpine AS frontend
WORKDIR /app/web/admin
COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci
COPY web/admin/ .
RUN npx next build --webpack

FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/admin/out ./web/admin/out
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o clawhost .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/clawhost .
EXPOSE 18080
CMD ["./clawhost", "server"]
