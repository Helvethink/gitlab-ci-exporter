FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG PROJECT_NAME=gitlab-ci-exporter
ARG PROJECT_PATH="./cmd/gitlab-ci-exporter"
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ENV CGO_ENABLED=0

WORKDIR /src

COPY . .
RUN go build -o ${PROJECT_NAME} -ldflags="-s -w -X main.version=${VERSION}" ${PROJECT_PATH}/main.go

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

COPY --from=builder /src/${PROJECT_NAME} /usr/local/bin/${PROJECT_NAME}

USER 33092
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/gitlab-ci-exporter"]
CMD ["run"]