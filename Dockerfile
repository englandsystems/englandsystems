FROM golang:1.26-alpine
WORKDIR /app
COPY . .
RUN go install
EXPOSE 9944
CMD ["./englandsystems"]

