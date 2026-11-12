package controller

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"

	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"google.golang.org/grpc/credentials"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
)

// otlpTransportOption returns the OTLP/gRPC transport security option for cfg.
//
// TLS is the default. Plaintext requires opentelemetry.insecure: true, which is
// intended for local development and logs a warning for non-loopback collectors.
func otlpTransportOption(cfg config.OpenTelemetry) (otlptracegrpc.Option, error) {
	if cfg.Insecure {
		if cfg.TLS != (config.OpenTelemetryTLS{}) {
			return nil, errors.New("opentelemetry.insecure cannot be combined with opentelemetry.tls settings")
		}
		if !isLoopbackEndpoint(cfg.GRPCEndpoint) {
			log.WithField("open-telemetry_grpc_endpoint", config.RedactURL(cfg.GRPCEndpoint)).
				Warn("open-telemetry traces are sent in plaintext to a non-loopback collector; use TLS outside local development")
		}
		return otlptracegrpc.WithInsecure(), nil
	}

	tlsConfig, err := otlpTLSConfig(cfg.TLS)
	if err != nil {
		return nil, err
	}
	return otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)), nil
}

func otlpTLSConfig(cfg config.OpenTelemetryTLS) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: cfg.ServerName,
	}

	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read opentelemetry CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("opentelemetry CA file contains no PEM certificate")
		}
		tlsConfig.RootCAs = pool
	}

	if (cfg.CertFile == "") != (cfg.KeyFile == "") {
		return nil, errors.New("opentelemetry.tls.cert_file and opentelemetry.tls.key_file must be set together")
	}
	if cfg.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load opentelemetry client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// isLoopbackEndpoint reports whether a host:port endpoint targets the local machine.
func isLoopbackEndpoint(endpoint string) bool {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
