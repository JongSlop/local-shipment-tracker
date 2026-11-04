FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /tracker .

FROM alpine:3
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /tracker /tracker
ENV DB_PATH=/data/tracker.db TZ=Europe/Berlin
VOLUME /data
EXPOSE 8080
CMD ["/tracker"]
