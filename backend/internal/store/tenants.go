package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UpsertTenant creates a tenant by slug or updates its name/OIDC settings.
func (s *Store) UpsertTenant(ctx context.Context, t model.Tenant) (model.Tenant, error) {
	now := time.Now().UTC()
	if t.DefaultRole == "" {
		t.DefaultRole = model.RoleMember
	}
	update := bson.M{
		"$set": bson.M{"updatedAt": now},
		"$setOnInsert": bson.M{
			"slug":        t.Slug,
			"name":        t.Name,
			"createdAt":   now,
			"defaultRole": t.DefaultRole,
		},
	}
	if t.OIDC != nil {
		update["$set"].(bson.M)["oidc"] = t.OIDC
	}
	if t.AllowedDomains != nil {
		update["$set"].(bson.M)["allowedDomains"] = t.AllowedDomains
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var out model.Tenant
	err := s.Tenants.FindOneAndUpdate(ctx, bson.M{"slug": t.Slug}, update, opts).Decode(&out)
	if err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) TenantBySlug(ctx context.Context, slug string) (model.Tenant, error) {
	var t model.Tenant
	err := s.Tenants.FindOne(ctx, bson.M{"slug": strings.ToLower(strings.TrimSpace(slug))}).Decode(&t)
	return t, err
}

func (s *Store) TenantByID(ctx context.Context, id primitive.ObjectID) (model.Tenant, error) {
	var t model.Tenant
	err := s.Tenants.FindOne(ctx, bson.M{"_id": id}).Decode(&t)
	return t, err
}

// ListTenants returns the public login directory (no secrets).
func (s *Store) ListTenants(ctx context.Context) ([]model.Tenant, error) {
	cur, err := s.Tenants.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var out []model.Tenant
	err = cur.All(ctx, &out)
	return out, err
}

func (s *Store) UpdateTenant(ctx context.Context, id primitive.ObjectID, set bson.M) (model.Tenant, error) {
	now := time.Now().UTC()
	set["updatedAt"] = now
	var t model.Tenant
	err := s.Tenants.FindOneAndUpdate(ctx, bson.M{"_id": id}, bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&t)
	return t, err
}

// ---- users ----

// UpsertOIDCUser finds a user by subject (or links by a verified email) and
// creates the membership if needed. The first member of a tenant becomes owner.
func (s *Store) UpsertOIDCUser(ctx context.Context, tenantID primitive.ObjectID, subject, email, name, avatar string, defaultRole string) (model.User, bool, error) {
	now := time.Now().UTC()
	email = strings.ToLower(strings.TrimSpace(email))
	filter := bson.M{"tenantId": tenantID, "subject": subject}
	var existing model.User
	err := s.Users.FindOne(ctx, filter).Decode(&existing)
	if err == nil {
		_, _ = s.Users.UpdateOne(ctx, filter, bson.M{"$set": bson.M{
			"email": email, "name": name, "avatarUrl": avatar, "lastLoginAt": now,
		}})
		existing.Email, existing.Name, existing.AvatarURL, existing.LastLoginAt = email, name, avatar, now
		return existing, false, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return model.User{}, false, err
	}
	// Link an existing dev/email user to the OIDC subject, unless another
	// subject already owns that email.
	if email != "" {
		if u, e := s.userByEmail(ctx, tenantID, email); e == nil {
			_, _ = s.Users.UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{"$set": bson.M{
				"subject": subject, "name": name, "avatarUrl": avatar, "lastLoginAt": now,
			}})
			u.Subject, u.Name, u.AvatarURL, u.LastLoginAt = subject, name, avatar, now
			return u, false, nil
		}
	}
	role := defaultRole
	if role == "" {
		role = model.RoleMember
	}
	count, err := s.Users.CountDocuments(ctx, bson.M{"tenantId": tenantID})
	if err != nil {
		return model.User{}, false, err
	}
	if count == 0 {
		role = model.RoleOwner
	}
	u := model.User{
		TenantID: tenantID, Subject: subject, Email: email, Name: name,
		AvatarURL: avatar, Role: role, LastLoginAt: now, CreatedAt: now,
	}
	res, err := s.Users.InsertOne(ctx, u)
	if err != nil {
		if IsDuplicate(err) {
			var again model.User
			if e := s.Users.FindOne(ctx, filter).Decode(&again); e == nil {
				return again, false, nil
			}
		}
		return model.User{}, false, err
	}
	u.ID = res.InsertedID.(primitive.ObjectID)
	return u, true, nil
}

// DevUser upserts the local development identity for a tenant.
func (s *Store) DevUser(ctx context.Context, tenantID primitive.ObjectID, email, name string) (model.User, error) {
	now := time.Now().UTC()
	email = strings.ToLower(strings.TrimSpace(email))
	subject := "dev:" + email
	var u model.User
	err := s.Users.FindOne(ctx, bson.M{"tenantId": tenantID, "subject": subject}).Decode(&u)
	if err == nil {
		_, _ = s.Users.UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{"$set": bson.M{
			"email": email, "name": name, "lastLoginAt": now,
		}})
		u.Email, u.Name, u.LastLoginAt = email, name, now
		return u, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return u, err
	}
	// Reuse an email membership if present.
	if existing, e := s.userByEmail(ctx, tenantID, email); e == nil {
		_, _ = s.Users.UpdateOne(ctx, bson.M{"_id": existing.ID}, bson.M{"$set": bson.M{
			"lastLoginAt": now, "name": name,
		}})
		existing.LastLoginAt, existing.Name = now, name
		return existing, nil
	}
	role := model.RoleMember
	count, err := s.Users.CountDocuments(ctx, bson.M{"tenantId": tenantID})
	if err != nil {
		return u, err
	}
	if count == 0 {
		role = model.RoleOwner
	}
	u = model.User{
		TenantID: tenantID, Subject: subject, Email: email, Name: name,
		Role: role, LastLoginAt: now, CreatedAt: now,
	}
	res, err := s.Users.InsertOne(ctx, u)
	if err != nil {
		return u, err
	}
	u.ID = res.InsertedID.(primitive.ObjectID)
	return u, nil
}

func (s *Store) userByEmail(ctx context.Context, tenantID primitive.ObjectID, email string) (model.User, error) {
	var u model.User
	err := s.Users.FindOne(ctx, bson.M{"tenantId": tenantID, "email": email}).Decode(&u)
	return u, err
}

func (s *Store) UserByID(ctx context.Context, tenantID, id primitive.ObjectID) (model.User, error) {
	var u model.User
	err := s.Users.FindOne(ctx, bson.M{"_id": id, "tenantId": tenantID}).Decode(&u)
	return u, err
}

func (s *Store) ListUsers(ctx context.Context, tenantID primitive.ObjectID) ([]model.User, error) {
	cur, err := s.Users.Find(ctx, bson.M{"tenantId": tenantID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var out []model.User
	err = cur.All(ctx, &out)
	return out, err
}

func (s *Store) UpdateUserRole(ctx context.Context, tenantID, userID primitive.ObjectID, role string) (model.User, error) {
	var u model.User
	err := s.Users.FindOneAndUpdate(ctx,
		bson.M{"_id": userID, "tenantId": tenantID},
		bson.M{"$set": bson.M{"role": role}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&u)
	return u, err
}

func (s *Store) DeleteUser(ctx context.Context, tenantID, userID primitive.ObjectID) error {
	_, err := s.Users.DeleteOne(ctx, bson.M{"_id": userID, "tenantId": tenantID})
	return err
}

func (s *Store) CountOwners(ctx context.Context, tenantID primitive.ObjectID) (int64, error) {
	return s.Users.CountDocuments(ctx, bson.M{"tenantId": tenantID, "role": model.RoleOwner})
}

// ---- sessions ----

func (s *Store) CreateSession(ctx context.Context, session model.Session) error {
	_, err := s.Sessions.InsertOne(ctx, session)
	return err
}

func (s *Store) SessionByTokenHash(ctx context.Context, hash string) (model.Session, error) {
	var sess model.Session
	err := s.Sessions.FindOne(ctx, bson.M{"tokenHash": hash, "expiresAt": bson.M{"$gt": time.Now().UTC()}}).Decode(&sess)
	return sess, err
}

func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, err := s.Sessions.DeleteOne(ctx, bson.M{"tokenHash": hash})
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, tenantID, userID primitive.ObjectID) error {
	_, err := s.Sessions.DeleteMany(ctx, bson.M{"tenantId": tenantID, "userId": userID})
	return err
}

// ---- OIDC state ----

func (s *Store) CreateOIDCState(ctx context.Context, st model.OIDCState) error {
	_, err := s.OIDC.InsertOne(ctx, st)
	return err
}

// ConsumeOIDCState atomically returns and deletes a login state.
func (s *Store) ConsumeOIDCState(ctx context.Context, hash string) (model.OIDCState, error) {
	var st model.OIDCState
	err := s.OIDC.FindOneAndDelete(ctx, bson.M{
		"stateHash": hash,
		"expiresAt": bson.M{"$gt": time.Now().UTC()},
	}).Decode(&st)
	return st, err
}
