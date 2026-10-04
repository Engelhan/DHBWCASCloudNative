// Package kafkatls builds the mTLS config for consumer and DLT producer.
package kafkatls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"ticketflow/notification-worker/internal/config"
)

// Build returns a *tls.Config from PEM values, or nil (plaintext) when KAFKA_TLS_ENABLED is false.
func Build(cfg config.Config) (*tls.Config, error) {
	if !cfg.KafkaTLSEnabled {
		return nil, nil
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(cfg.KafkaTLSCACert)) {
		return nil, fmt.Errorf("KAFKA_TLS_CA_CERT: no valid PEM certificate found")
	}

	cert, err := tls.X509KeyPair([]byte(cfg.KafkaTLSClientCert), []byte(cfg.KafkaTLSClientKey))
	if err != nil {
		return nil, fmt.Errorf("parse KAFKA_TLS_CLIENT_CERT/KAFKA_TLS_CLIENT_KEY: %w", err)
	}

	return &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
	}, nil
}
