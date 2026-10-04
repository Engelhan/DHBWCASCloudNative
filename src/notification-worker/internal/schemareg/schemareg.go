// Package schemareg is a minimal Apicurio client: it resolves an artifact's globalId at startup,
// strips the Confluent-style header (magic byte + 4-byte globalId) and validates payloads against
// an embedded JSON schema. No network call per message.
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
	// BaseURL is the v3 REST root, e.g. http://apicurio.kafka.svc:8080/apis/registry/v3.
	BaseURL    string
	GroupID    string
	ArtifactID string

	// Logger is optional.
	Logger *slog.Logger
}

type Client struct {
	cfg      Config
	http     *http.Client
	schema   *jsonschema.Schema
	globalID atomic.Int64 // 0 until resolved
}

// New compiles the schema for local validation; no network call.
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

// Resolve fetches the current globalId, retrying until ctx is done.
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

// Ping re-resolves lazily so /readyz recovers once the registry is reachable.
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

// Unwrap strips the header and validates against the embedded schema. A different globalId
// (schema evolved) is only logged.
func (c *Client) Unwrap(msg []byte) ([]byte, error) {
	if len(msg) < frameHeaderSz {
		return nil, fmt.Errorf("schemareg: message too short for framing header (%d bytes)", len(msg))
	}
	if msg[0] != magicByte {
		return nil, fmt.Errorf("schemareg: unexpected magic byte 0x%x", msg[0])
	}

	gotID := int64(binary.BigEndian.Uint32(msg[1:frameHeaderSz]))
	if wantID := c.globalID.Load(); wantID != 0 && gotID != wantID {
		c.warn("message framed with a different globalId than currently resolved (schema may have evolved)",
			"artifactId", c.cfg.ArtifactID, "messageGlobalId", gotID, "resolvedGlobalId", wantID)
	}

	payload := msg[frameHeaderSz:]
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("schemareg: parse payload as JSON: %w", err)
	}
	if err := c.schema.Validate(instance); err != nil {
		return nil, fmt.Errorf("schemareg: payload violates %s schema: %w", c.cfg.ArtifactID, err)
	}
	return payload, nil
}

func (c *Client) warn(msg string, args ...any) {
	if c.cfg.Logger != nil {
		c.cfg.Logger.Warn(msg, args...)
	}
}
