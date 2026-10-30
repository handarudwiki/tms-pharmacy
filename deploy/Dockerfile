# Builds one TMS service: docker build --build-arg SERVICE=planning -f deploy/Dockerfile .
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npx vite build

FROM golang:1.25-alpine AS build
ARG SERVICE
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./services/${SERVICE}

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/app /app
COPY --from=web /web/dist /web
ENV WEB_DIR=/web
ENTRYPOINT ["/app"]
