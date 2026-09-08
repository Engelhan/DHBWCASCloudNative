// Package kafkatls builds the mutual-TLS configuration shared by the Kafka
// consumer and the dead-letter-topic producer.
package kafkatls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"ticketflow/notification-worker/internal/config"
)

// Build constructs a *tls.Config for mTLS to the Kafka brokers from
// PEM-encoded CA/client-cert/client-key values. Returns (nil, nil) when
// KAFKA_TLS_ENABLED is false, so callers can pass the result straight to
// kafka.Dialer{TLS: ...}/kafka.Transport{TLS: ...} either way -- a nil
// *tls.Config there means "plaintext", which is the current default (no
// Strimzi listener/SASL config exists yet in this repo).
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
