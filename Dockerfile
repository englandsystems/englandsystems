FROM golang:1.26-alpine

WORKDIR /app

RUN apk add --no-cache build-base

COPY . .

RUN go build -o englandsystems .

EXPOSE 9944

CMD ["./englandsystems"]
