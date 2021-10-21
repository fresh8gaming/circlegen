FROM golang:1.17-alpine AS builder

WORKDIR $GOPATH/src/mypackage/myapp/

COPY . .

RUN pwd
RUN ls -la

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s -extldflags \"-static\"" -a -o /go/bin/circleci-config-generator ./main.go

FROM cimg/go:1.17

COPY --from=builder /go/bin/circleci-config-generator /usr/local/go/bin/circleci-config-generator
