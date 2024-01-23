FROM golang:latest

WORKDIR /app

COPY go.mod go.sum config.json ./
COPY *.go ./
COPY certs ./certs/

RUN go mod download
RUN go build -o ./out/app *.go

CMD [ "./out/app" ]