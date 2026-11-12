package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helvethink/gitlab-ci-exporter/pkg/config"
)

// writeTestKeyPair writes a self-signed certificate and its key as PEM files.
func writeTestKeyPair(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "collector.test"},
		DNSNames:              []string{"collector.test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certFile, keyFile
}

func TestOTLPTLSConfigDefaultsToSystemRoots(t *testing.T) {
	tlsConfig, err := otlpTLSConfig(config.OpenTelemetryTLS{})
	require.NoError(t, err)
	assert.Nil(t, tlsConfig.RootCAs, "nil RootCAs means the system pool is used")
	assert.Empty(t, tlsConfig.Certificates)
	assert.False(t, tlsConfig.InsecureSkipVerify)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
}

func TestOTLPTLSConfigLoadsCAClientCertificateAndServerName(t *testing.T) {
	certFile, keyFile := writeTestKeyPair(t)

	tlsConfig, err := otlpTLSConfig(config.OpenTelemetryTLS{
		CAFile:     certFile,
		CertFile:   certFile,
		KeyFile:    keyFile,
		ServerName: "collector.test",
	})
	require.NoError(t, err)
	assert.NotNil(t, tlsConfig.RootCAs)
	assert.Len(t, tlsConfig.Certificates, 1)
	assert.Equal(t, "collector.test", tlsConfig.ServerName)
}

func TestOTLPTLSConfigRejectsInvalidSettings(t *testing.T) {
	certFile, keyFile := writeTestKeyPair(t)
	notPEM := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(notPEM, []byte("not a certificate"), 0o600))

	tests := map[string]config.OpenTelemetryTLS{
		"missing CA file":     {CAFile: filepath.Join(t.TempDir(), "missing.pem")},
		"CA without PEM":      {CAFile: notPEM},
		"cert without key":    {CertFile: certFile},
		"key without cert":    {KeyFile: keyFile},
		"mismatched key pair": {CertFile: certFile, KeyFile: certFile},
	}
	for name, tlsSettings := range tests {
		_, err := otlpTLSConfig(tlsSettings)
		assert.Error(t, err, name)
	}
}

func TestOTLPTransportRejectsInsecureWithTLSSettings(t *testing.T) {
	_, err := otlpTransportOption(config.OpenTelemetry{
		GRPCEndpoint: "localhost:4317",
		Insecure:     true,
		TLS:          config.OpenTelemetryTLS{ServerName: "collector.test"},
	})
	require.ErrorContains(t, err, "cannot be combined")
}

func TestIsLoopbackEndpoint(t *testing.T) {
	for endpoint, want := range map[string]bool{
		"localhost:4317":      true,
		"127.0.0.1:4317":      true,
		"[::1]:4317":          true,
		"otel-collector:4317": false,
		"10.0.0.5:4317":       false,
		"collector.test":      false,
	} {
		assert.Equal(t, want, isLoopbackEndpoint(endpoint), endpoint)
	}
}

func TestConfigureTracingDoesNotBlockOnUnreachableCollector(t *testing.T) {
	for name, cfg := range map[string]config.OpenTelemetry{
		"tls":      {GRPCEndpoint: "127.0.0.1:1"},
		"insecure": {GRPCEndpoint: "127.0.0.1:1", Insecure: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			start := time.Now()
			provider, err := configureTracing(ctx, cfg)
			require.NoError(t, err)
			require.NotNil(t, provider)
			assert.Less(t, time.Since(start), 2*time.Second, "startup must not wait for the collector")

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
			defer shutdownCancel()
			_ = provider.Shutdown(shutdownCtx)
		})
	}
}
