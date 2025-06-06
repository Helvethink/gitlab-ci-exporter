FROM golang:1.24 AS builder

ARG PROJECT_NAME=gitlab-ci-exporter
ARG PROJECT_PATH="cmd/gitlab-ci-exporter"

WORKDIR /app/

COPY . .
RUN echo "CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o ${PROJECT_NAME} ${PROJECT_PATH}/main.go"
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o ${PROJECT_NAME} ${PROJECT_PATH}/main.go

FROM alpine:3.22

WORKDIR /app/
COPY --from=builder /app/${PROJECT_NAME} /app/${PROJECT_NAME}

RUN chmod +x /app/${PROJECT_NAME}

USER 33092
EXPOSE 8080

ENTRYPOINT ["/app/gitlab-ci-exporter"]
CMD ["run"]