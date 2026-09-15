// Package schemareg is a minimal client for Apicurio Registry (charts/apicurio). It resolves the
// current globalId of one registry artifact once at startup, frames outgoing message payloads
// with a Confluent-compatible header (0x0 magic byte + 4-byte big-endian globalId), and validates
// payloads against a JSON Schema embedded in the binary at build time (embedded.go) -- Wrap never
// makes a network call per message, only Resolve/Ping do, and only against the registry's
// "latest version" metadata endpoint. This keeps message throughput independent of registry
// availability once a Client has successfully resolved once.
//
// Not a general-purpose Apicurio client: it only implements the two calls this repo's services
// need (create-or-update is done by the charts/kafka-cluster schema-registrar Job, not from app
// code -- see that Job's comment for why). Duplicated per service rather than shared via a Go
// module, same convention as reservation-worker/notification-worker's internal/event packages.
package schemareg

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	magicByte     byte = 0x0
	frameHeaderSz      = 5 // 1 magic byte + 4-byte big-endian globalId
)

type Config struct {
	// BaseURL is the registry's v3 REST API root, e.g.
	// "http://apicurio.kafka.svc:8080/apis/registry/v3" (no trailing slash).
	BaseURL    string
	GroupID    string
	ArtifactID string

	// Logger is optional; nil disables logging (e.g. in tests).
	Logger *slog.Logger
}

type Client struct {
	cfg      Config
	http     *http.Client
	schema   *jsonschema.Schema
	globalID atomic.Int64 // 0 until Resolve/Ping succeeds at least once
}

// New compiles schemaJSON (one of the embedded.go vars) for local validation. It performs no
// network call -- call Resolve before the client is used to Wrap, or rely on Ping (wired into
// /readyz) to resolve it lazily.
func New(cfg Config, schemaJSON []byte) (*Client, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("schemareg: parse embedded schema for %s: %w", cfg.ArtifactID, err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(cfg.ArtifactID, doc); err != nil {
		return nil, fmt.Errorf("schemareg: add embedded schema resource for %s: %w", cfg.ArtifactID, err)
	}
	schema, err := compiler.Compile(cfg.ArtifactID)
	if err != nil {
		return nil, fmt.Errorf("schemareg: compile embedded schema for %s: %w", cfg.ArtifactID, err)
	}
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 5 * time.Second},
		schema: schema,
	}, nil
}

// Resolve blocks until the artifact's current globalId has been fetched from the registry,
// retrying indefinitely until ctx is done. Call it once at startup before serving traffic.
func (c *Client) Resolve(ctx context.Context) error {
	for {
		gid, err := c.fetchGlobalID(ctx)
		if err == nil {
			c.globalID.Store(gid)
			return nil
		}
		c.warn("schema registry artifact not yet resolvable, retrying", "artifactId", c.cfg.ArtifactID, "error", err)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Ping satisfies the Pinger interface already used for DB/Kafka reachability (see
// internal/api). It re-resolves lazily if Resolve was never called or the registry was
// unreachable at startup, so /readyz recovers once the registry becomes reachable.
func (c *Client) Ping(ctx context.Context) error {
	if c.globalID.Load() != 0 {
		return nil
	}
	gid, err := c.fetchGlobalID(ctx)
	if err != nil {
		return err
	}
	c.globalID.Store(gid)
	return nil
}

func (c *Client) fetchGlobalID(ctx context.Context) (int64, error) {
	url := fmt.Sprintf("%s/groups/%s/artifacts/%s/versions/branch=latest", c.cfg.BaseURL, c.cfg.GroupID, c.cfg.ArtifactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf("unexpected status %d fetching %s: %s", resp.StatusCode, url, body)
	}

	// globalId's exact JSON field name/casing is not yet verified against a live registry -- see
	// charts/README.md ## apicurio. Adjust here if a real response differs.
	var meta struct {
		GlobalID int64 `json:"globalId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return 0, fmt.Errorf("decode version metadata: %w", err)
	}
	if meta.GlobalID == 0 {
		return 0, fmt.Errorf("version metadata response missing globalId")
	}
	return meta.GlobalID, nil
}

// Wrap validates payload against the embedded schema, then frames it with the registry header.
// Returns an error (not a panic/os.Exit) on either a validation failure or an unresolved
// globalId -- callers integrate this the same way they already handle other publish failures,
// see the call site in internal/reservation/producer.go.
func (c *Client) Wrap(payload []byte) ([]byte, error) {
	if err := c.validate(payload); err != nil {
		return nil, err
	}
	gid := c.globalID.Load()
	if gid == 0 {
		return nil, fmt.Errorf("schemareg: globalId not yet resolved for %s", c.cfg.ArtifactID)
	}

	framed := make([]byte, frameHeaderSz, frameHeaderSz+len(payload))
	framed[0] = magicByte
	binary.BigEndian.PutUint32(framed[1:frameHeaderSz], uint32(gid))
	framed = append(framed, payload...)
	return framed, nil
}

// catalog-api only produces -- no Unwrap here. reservation-worker (which both consumes and
// produces) and notification-worker (which only consumes) carry the full Wrap+Unwrap
// implementation in their own copies of this package.

func (c *Client) validate(payload []byte) error {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("schemareg: parse payload as JSON: %w", err)
	}
	if err := c.schema.Validate(instance); err != nil {
		return fmt.Errorf("schemareg: payload violates %s schema: %w", c.cfg.ArtifactID, err)
	}
	return nil
}

func (c *Client) warn(msg string, args ...any) {
	if c.cfg.Logger != nil {
		c.cfg.Logger.Warn(msg, args...)
	}
}
