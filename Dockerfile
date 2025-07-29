FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build for Windows 64-bit
RUN GOOS=windows GOARCH=amd64 go build -o /app/driving-exam-slots-searcher-win.exe .
# Build for Linux 64-bit
RUN GOOS=linux GOARCH=amd64 go build -o /app/driving-exam-slots-searcher-lnx .

FROM scratch
WORKDIR /exam-slots-searcher
COPY --from=builder /app/driving-exam-slots-searcher-win.exe .
COPY --from=builder /app/driving-exam-slots-searcher-lnx .
COPY --from=builder /app/config.yaml .
COPY --from=builder /app/secret.txt .
CMD ["/exam-slots-searcher/driving-exam-slots-searcher.exe"]
