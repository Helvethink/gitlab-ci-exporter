FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.23 as builder

ARG PROJECT_NAME=generator
ARG PROJECT_PATH=generator
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH


WORKDIR /app/
COPY . .
RUN echo "CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o ${PROJECT_NAME} ${PROJECT_PATH}/main.go"

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o ${PROJECT_NAME} ${PROJECT_PATH}/main.go

FROM --platform=${TARGETPLATFORM:-linux/amd64} alpine:3.21.2
WORKDIR /app/


COPY --from=builder /app/${PROJECT_NAME} /app/${PROJECT_NAME}
RUN ls -ltrh generator/
RUN chmod +x /app/${PROJECT_NAME}

ENTRYPOINT ["/app/generator"]