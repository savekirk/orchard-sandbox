# Dashboard
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Server (pure Go, no CGO)
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /orchard-sandbox .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /orchard-sandbox /orchard-sandbox
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=2s CMD ["/orchard-sandbox", "-healthcheck"]
ENTRYPOINT ["/orchard-sandbox"]
