FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG PROJECT_NAME=gitlab-ci-exporter
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ENV CGO_ENABLED=0

WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY . .
# RUN go build -o /out/${PROJECT_NAME} -ldflags="-s -w -X main.version=${VERSION}" ./cmd/${PROJECT_NAME}/main.go

FROM scratch

# COPY --from=builder /out/${PROJECT_NAME} /usr/local/bin/${PROJECT_NAME}
COPY gitlab-ci-exporter /gitlab-ci-exporter
USER 33092
EXPOSE 8080

ENTRYPOINT ["/gitlab-ci-exporter"]
CMD ["run"]