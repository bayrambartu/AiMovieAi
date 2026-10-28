FROM node:22-alpine AS web
WORKDIR /web
COPY frontend/package*.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

FROM golang:1.22-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D appuser
WORKDIR /app
COPY --from=api /server ./server
COPY --from=web /web/dist ./web
USER appuser
ENV ADDR=0.0.0.0:8080 FRONTEND_DIST=/app/web
EXPOSE 8080
CMD ["./server"]
