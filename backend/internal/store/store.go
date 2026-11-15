package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Store is the MongoDB persistence layer.
type Store struct {
	Client     *mongo.Client
	DB         *mongo.Database
	Tenants    *mongo.Collection
	Users      *mongo.Collection
	Sessions   *mongo.Collection
	OIDC       *mongo.Collection
	Projects   *mongo.Collection
	Events     *mongo.Collection
	Issues     *mongo.Collection
	Comments   *mongo.Collection
	SourceMaps *mongo.Collection
	Spans      *mongo.Collection
	Metrics    *mongo.Collection
}

// Connect opens a MongoDB connection and ensures indexes exist.
func Connect(ctx context.Context, uri, dbName string, retention time.Duration) (*Store, error) {
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI(uri).
		SetAppName("catch-api").
		SetMaxPoolSize(50).
		SetMinPoolSize(2).
		SetConnectTimeout(10*time.Second))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	db := client.Database(dbName)
	s := &Store{
		Client:     client,
		DB:         db,
		Tenants:    db.Collection("tenants"),
		Users:      db.Collection("users"),
		Sessions:   db.Collection("sessions"),
		OIDC:       db.Collection("oidc_states"),
		Projects:   db.Collection("projects"),
		Events:     db.Collection("events"),
		Issues:     db.Collection("issues"),
		Comments:   db.Collection("comments"),
		SourceMaps: db.Collection("source_maps"),
		Spans:      db.Collection("spans"),
		Metrics:    db.Collection("metrics"),
	}
	if err := s.ensureIndexes(ctx, retention); err != nil {
		return nil, err
	}
	return s, nil
}

// Close disconnects from MongoDB.
func (s *Store) Close(ctx context.Context) error {
	if s == nil || s.Client == nil {
		return nil
	}
	return s.Client.Disconnect(ctx)
}

func (s *Store) ensureIndexes(ctx context.Context, retention time.Duration) error {
	type spec struct {
		coll string
		keys bson.D
		opts *options.IndexOptions
	}
	idx := func(name string) *options.IndexOptions { return options.Index().SetName(name) }
	specs := []spec{
		{"tenants", bson.D{{Key: "slug", Value: 1}}, idx("uniq_tenant_slug").SetUnique(true)},
		{"users", bson.D{{Key: "tenantId", Value: 1}, {Key: "subject", Value: 1}}, idx("uniq_user_subject").SetUnique(true)},
		{"users", bson.D{{Key: "tenantId", Value: 1}, {Key: "email", Value: 1}}, idx("uniq_user_email").SetUnique(true)},
		{"sessions", bson.D{{Key: "tokenHash", Value: 1}}, idx("uniq_session_token").SetUnique(true)},
		{"sessions", bson.D{{Key: "expiresAt", Value: 1}}, idx("ttl_session").SetExpireAfterSeconds(0)},
		{"oidc_states", bson.D{{Key: "stateHash", Value: 1}}, idx("uniq_oidc_state").SetUnique(true)},
		{"oidc_states", bson.D{{Key: "expiresAt", Value: 1}}, idx("ttl_oidc_state").SetExpireAfterSeconds(0)},
		{"projects", bson.D{{Key: "tenantId", Value: 1}, {Key: "slug", Value: 1}}, idx("uniq_project_slug").SetUnique(true)},
		{"projects", bson.D{{Key: "ingestKey", Value: 1}}, idx("uniq_project_key").SetUnique(true)},
		{"events", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "kind", Value: 1}, {Key: "timestamp", Value: -1}}, idx("events_project_kind_time")},
		{"events", bson.D{{Key: "tenantId", Value: 1}, {Key: "source", Value: 1}, {Key: "kind", Value: 1}, {Key: "timestamp", Value: -1}}, idx("events_source_kind_time")},
		{"events", bson.D{{Key: "tenantId", Value: 1}, {Key: "kind", Value: 1}, {Key: "timestamp", Value: -1}}, idx("events_tenant_kind_time")},
		{"events", bson.D{{Key: "issueId", Value: 1}, {Key: "timestamp", Value: -1}}, idx("events_issue_time")},
		{"issues", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "fingerprint", Value: 1}}, idx("uniq_issue_fingerprint").SetUnique(true)},
		{"issues", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "status", Value: 1}, {Key: "lastSeen", Value: -1}}, idx("issues_status_lastseen")},
		{"issues", bson.D{{Key: "tenantId", Value: 1}, {Key: "lastSeen", Value: -1}}, idx("issues_tenant_lastseen")},
		{"comments", bson.D{{Key: "issueId", Value: 1}, {Key: "createdAt", Value: 1}}, idx("comments_issue_time")},
		{"source_maps", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "release", Value: 1}, {Key: "file", Value: 1}}, idx("uniq_sourcemap").SetUnique(true)},
		{"source_maps", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "release", Value: 1}, {Key: "updatedAt", Value: -1}}, idx("sourcemaps_release")},
		{"spans", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "traceId", Value: 1}, {Key: "spanId", Value: 1}}, idx("uniq_span").SetUnique(true)},
		{"spans", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "traceId", Value: 1}, {Key: "startTime", Value: 1}}, idx("spans_trace")},
		{"spans", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "startTime", Value: -1}}, idx("spans_start")},
		{"spans", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "service", Value: 1}, {Key: "startTime", Value: -1}}, idx("spans_service")},
		{"metrics", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "name", Value: 1}, {Key: "timestamp", Value: -1}}, idx("metrics_name_time")},
		{"metrics", bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "timestamp", Value: -1}}, idx("metrics_time")},
	}
	for _, sp := range specs {
		coll := s.DB.Collection(sp.coll)
		if _, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: sp.keys, Options: sp.opts}); err != nil {
			// A conflicting index with the same key but different options can
			// fail; surface anything that is not an existing-name conflict.
			if !mongo.IsDuplicateKeyError(err) {
				return fmt.Errorf("create index %v on %s: %w", sp.opts.Name, sp.coll, err)
			}
		}
	}
	if retention > 0 {
		if err := s.ensureTTL(ctx, s.Events, "events", "timestamp", "ttl_event", retention); err != nil {
			return err
		}
		if err := s.ensureTTL(ctx, s.Spans, "spans", "startTime", "ttl_span", retention); err != nil {
			return err
		}
		if err := s.ensureTTL(ctx, s.Metrics, "metrics", "timestamp", "ttl_metric", retention); err != nil {
			return err
		}
	}
	return nil
}

// ensureTTL keeps a collection TTL aligned with RETENTION_DAYS even when the
// collection already exists from a previous deploy.
func (s *Store) ensureTTL(ctx context.Context, coll *mongo.Collection, collName, field, indexName string, retention time.Duration) error {
	seconds := int32(retention.Seconds())
	cursor, err := coll.Indexes().List(ctx)
	if err != nil {
		return fmt.Errorf("list %s indexes: %w", collName, err)
	}
	defer cursor.Close(ctx)
	var found bool
	for cursor.Next(ctx) {
		var item bson.M
		if err := cursor.Decode(&item); err != nil {
			continue
		}
		keys, _ := item["key"].(bson.M)
		if keys == nil {
			if d, ok := item["key"].(bson.D); ok {
				keys = bson.M{}
				for _, e := range d {
					keys[e.Key] = e.Value
				}
			}
		}
		if v, ok := keys[field]; ok && fmt.Sprint(v) == "1" {
			found = true
			if exp, ok := item["expireAfterSeconds"].(int32); ok && exp == seconds {
				return nil
			}
			if exp, ok := item["expireAfterSeconds"].(int64); ok && exp == int64(seconds) {
				return nil
			}
			_ = s.DB.RunCommand(ctx, bson.D{
				{Key: "collMod", Value: collName},
				{Key: "index", Value: bson.D{
					{Key: "keyPattern", Value: bson.D{{Key: field, Value: 1}}},
					{Key: "expireAfterSeconds", Value: seconds},
				}},
			}).Err()
			return nil
		}
	}
	if found {
		return nil
	}
	opts := options.Index().SetName(indexName).SetExpireAfterSeconds(seconds)
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: field, Value: 1}},
		Options: opts,
	})
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("create %s ttl: %w", collName, err)
	}
	return nil
}

// IsDuplicate reports whether err is a Mongo duplicate-key error.
func IsDuplicate(err error) bool { return mongo.IsDuplicateKeyError(err) }

// IsNoDocuments reports whether err means no matching document.
func IsNoDocuments(err error) bool { return errors.Is(err, mongo.ErrNoDocuments) }
