package migrate

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoStore keeps applied versions in the schema_migrations collection.
type MongoStore struct{ coll *mongo.Collection }

func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{coll: db.Collection("schema_migrations")}
}

func (s *MongoStore) Applied(ctx context.Context) (map[int]bool, error) {
	cur, err := s.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := map[int]bool{}
	for cur.Next(ctx) {
		var d struct {
			ID int32 `bson:"_id"`
		}
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out[int(d.ID)] = true
	}
	return out, cur.Err()
}

func (s *MongoStore) Record(ctx context.Context, version int, name string) error {
	_, err := s.coll.UpdateOne(ctx, bson.M{"_id": int32(version)},
		bson.M{"$setOnInsert": bson.M{"name": name, "appliedAt": time.Now().UTC()}},
		options.Update().SetUpsert(true))
	return err
}

// Steps returns the ordered MongoDB migrations for db.
func Steps(db *mongo.Database) []Step {
	return []Step{
		{Version: 1, Name: "users_roles_status_username", Up: func(ctx context.Context) error { return usersStep(ctx, db) }},
		{Version: 2, Name: "hospitals_active", Up: func(ctx context.Context) error { return hospitalsStep(ctx, db) }},
		{Version: 3, Name: "cell_library", Up: func(ctx context.Context) error { return cellLibraryStep(ctx, db) }},
		{Version: 4, Name: "indexes", Up: func(ctx context.Context) error { return indexesStep(ctx, db) }},
	}
}

// usersStep reports (and stops on) duplicate usernames, then migrates roles,
// statuses and the lower-cased username. It never deletes or merges users.
func usersStep(ctx context.Context, db *mongo.Database) error {
	users := db.Collection("users")

	cur, err := users.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{"$toLower": "$username"}, "n": bson.M{"$sum": 1}, "ids": bson.M{"$push": "$_id"},
		}}},
		{{Key: "$match", Value: bson.M{"n": bson.M{"$gt": 1}}}},
	})
	if err != nil {
		return err
	}
	var dups []struct {
		Username string               `bson:"_id"`
		IDs      []primitive.ObjectID `bson:"ids"`
	}
	if err := cur.All(ctx, &dups); err != nil {
		return err
	}
	if len(dups) > 0 {
		var lines []string
		for _, d := range dups {
			ids := make([]string, len(d.IDs))
			for i, id := range d.IDs {
				ids[i] = id.Hex()
			}
			lines = append(lines, fmt.Sprintf("%q used by %s", d.Username, strings.Join(ids, ", ")))
		}
		sort.Strings(lines)
		return fmt.Errorf("duplicate usernames must be resolved by hand before migrating (no data was changed): %s", strings.Join(lines, "; "))
	}

	// instructor -> admin
	if _, err := users.UpdateMany(ctx, bson.M{"role": "instructor"}, bson.M{"$set": bson.M{"role": entity.RoleAdmin}}); err != nil {
		return err
	}
	// Status for accounts that predate it.
	noStatus := bson.M{"status": bson.M{"$exists": false}}
	staff := bson.M{"status": bson.M{"$exists": false}, "role": bson.M{"$in": []entity.Role{entity.RoleAdmin, entity.RoleSuperAdmin}}}
	if _, err := users.UpdateMany(ctx, staff, bson.M{"$set": bson.M{"status": entity.UserActive}}); err != nil {
		return err
	}
	noHospital := bson.M{"$or": []bson.M{
		{"hospitalId": bson.M{"$exists": false}}, {"hospitalId": nil}, {"hospitalId": primitive.NilObjectID},
	}}
	pending := bson.M{"$and": []bson.M{noStatus, {"role": entity.RoleUser}, noHospital}}
	if _, err := users.UpdateMany(ctx, pending, bson.M{"$set": bson.M{"status": entity.UserPending}}); err != nil {
		return err
	}
	if _, err := users.UpdateMany(ctx, noStatus, bson.M{"$set": bson.M{"status": entity.UserActive}}); err != nil {
		return err
	}
	// usernameLower (pipeline update needs MongoDB >= 4.2).
	_, err = users.UpdateMany(ctx,
		bson.M{"$or": []bson.M{{"usernameLower": bson.M{"$exists": false}}, {"usernameLower": ""}}},
		mongo.Pipeline{{{Key: "$set", Value: bson.M{"usernameLower": bson.M{"$toLower": "$username"}}}}})
	return err
}

func hospitalsStep(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("hospitals").UpdateMany(ctx,
		bson.M{"active": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"active": true}})
	return err
}

// cellLibraryStep builds cell_types from defaults, legacy quiz categories and
// legacy asset types, and copies cell_image_assets into cell_images. It only
// inserts (setOnInsert) so re-runs and admin edits are never overwritten, and
// it leaves the legacy collections untouched.
func cellLibraryStep(ctx context.Context, db *mongo.Database) error {
	now := time.Now().UTC()
	types := map[string]entity.CellType{}
	for _, t := range entity.DefaultCellTypes() {
		types[t.Key] = t
	}

	// Labels from legacy quiz categories.
	qc, err := db.Collection("quizzes").Find(ctx, bson.M{"categories": bson.M{"$exists": true}},
		options.Find().SetProjection(bson.M{"categories": 1}))
	if err != nil {
		return err
	}
	var quizzes []struct {
		Categories []entity.LegacyCategory `bson:"categories"`
	}
	if err := qc.All(ctx, &quizzes); err != nil {
		return err
	}
	next := 100
	for _, q := range quizzes {
		for _, c := range q.Categories {
			if c.Key == "" {
				continue
			}
			if _, ok := types[c.Key]; !ok {
				types[c.Key] = entity.CellType{Key: c.Key, Label: orLabel(c.Label, c.Key), SortOrder: next, Active: true}
				next += 10
			}
		}
	}

	assets := db.Collection("cell_image_assets")
	keys, err := assets.Distinct(ctx, "cellType", bson.M{})
	if err != nil {
		return err
	}
	var assetKeys []string
	for _, k := range keys {
		if s, ok := k.(string); ok && s != "" {
			assetKeys = append(assetKeys, s)
		}
	}
	sort.Strings(assetKeys)
	for _, k := range assetKeys {
		if _, ok := types[k]; !ok {
			types[k] = entity.CellType{Key: k, Label: orLabel("", k), SortOrder: next, Active: true}
			next += 10
		}
	}

	var typeOps []mongo.WriteModel
	for _, t := range types {
		typeOps = append(typeOps, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"key": t.Key}).SetUpsert(true).
			SetUpdate(bson.M{"$setOnInsert": bson.M{
				"key": t.Key, "label": t.Label, "sortOrder": t.SortOrder, "active": true, "createdAt": now,
			}}))
	}
	if len(typeOps) > 0 {
		if _, err := db.Collection("cell_types").BulkWrite(ctx, typeOps, options.BulkWrite().SetOrdered(false)); err != nil {
			return err
		}
	}

	// Copy assets -> images in batches.
	cur, err := assets.Find(ctx, bson.M{})
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	images := db.Collection("cell_images")
	var batch []mongo.WriteModel
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := images.BulkWrite(ctx, batch, options.BulkWrite().SetOrdered(false))
		batch = batch[:0]
		return err
	}
	for cur.Next(ctx) {
		var a struct {
			CellType  string    `bson:"cellType"`
			Path      string    `bson:"path"`
			CreatedAt time.Time `bson:"createdAt"`
		}
		if err := cur.Decode(&a); err != nil {
			return err
		}
		if a.Path == "" || a.CellType == "" {
			continue
		}
		created := a.CreatedAt
		if created.IsZero() {
			created = now
		}
		batch = append(batch, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"path": a.Path}).SetUpsert(true).
			SetUpdate(bson.M{"$setOnInsert": bson.M{
				"typeKey": a.CellType, "path": a.Path, "active": true, "createdAt": created,
			}}))
		if len(batch) >= 500 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := cur.Err(); err != nil {
		return err
	}
	return flush()
}

func orLabel(label, key string) string {
	if strings.TrimSpace(label) != "" {
		return label
	}
	if key == "" {
		return key
	}
	return strings.ToUpper(key[:1]) + key[1:]
}

type idx struct {
	coll   string
	name   string
	keys   bson.D
	unique bool
	filter bson.M
}

func indexesStep(ctx context.Context, db *mongo.Database) error {
	defs := []idx{
		{coll: "users", name: "uniq_usernameLower", keys: bson.D{{Key: "usernameLower", Value: 1}}, unique: true},
		{coll: "users", name: "hospitalId_status", keys: bson.D{{Key: "hospitalId", Value: 1}, {Key: "status", Value: 1}}},
		{coll: "users", name: "status_createdAt", keys: bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}},
		{coll: "hospitals", name: "uniq_code", keys: bson.D{{Key: "code", Value: 1}}, unique: true},
		{coll: "cell_types", name: "uniq_key", keys: bson.D{{Key: "key", Value: 1}}, unique: true},
		{coll: "cell_images", name: "uniq_path", keys: bson.D{{Key: "path", Value: 1}}, unique: true},
		{coll: "cell_images", name: "active_typeKey", keys: bson.D{{Key: "active", Value: 1}, {Key: "typeKey", Value: 1}}},
		{coll: "quizzes", name: "v_status_createdAt", keys: bson.D{{Key: "v", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}},
		{coll: "quiz_assignments", name: "uniq_quiz_hospital", keys: bson.D{{Key: "quizId", Value: 1}, {Key: "hospitalId", Value: 1}}, unique: true},
		{coll: "quiz_assignments", name: "hospital_status", keys: bson.D{{Key: "hospitalId", Value: 1}, {Key: "status", Value: 1}}},
		{coll: "attempts", name: "uniq_in_progress_quiz_user", keys: bson.D{{Key: "quizId", Value: 1}, {Key: "userId", Value: 1}},
			unique: true, filter: bson.M{"status": "in_progress"}},
		{coll: "attempts", name: "uniq_quiz_user_attemptNo", keys: bson.D{{Key: "quizId", Value: 1}, {Key: "userId", Value: 1}, {Key: "attemptNo", Value: 1}}, unique: true},
		{coll: "attempts", name: "quiz_hospital_status", keys: bson.D{{Key: "quizId", Value: 1}, {Key: "hospitalId", Value: 1}, {Key: "status", Value: 1}}},
		{coll: "attempts", name: "user_startedAt", keys: bson.D{{Key: "userId", Value: 1}, {Key: "startedAt", Value: -1}}},
		{coll: "certificates", name: "uniq_certNo", keys: bson.D{{Key: "certNo", Value: 1}}, unique: true},
		{coll: "certificates", name: "uniq_attemptId", keys: bson.D{{Key: "attemptId", Value: 1}}, unique: true},
		{coll: "certificates", name: "uniq_quiz_user", keys: bson.D{{Key: "quizId", Value: 1}, {Key: "userId", Value: 1}}, unique: true},
		{coll: "certificates", name: "user_issuedAt", keys: bson.D{{Key: "userId", Value: 1}, {Key: "issuedAt", Value: -1}}},
		{coll: "audit_logs", name: "createdAt", keys: bson.D{{Key: "createdAt", Value: -1}}},
	}
	for _, d := range defs {
		if err := dropConflictingIndexes(ctx, db.Collection(d.coll), d); err != nil {
			return err
		}
		opts := options.Index().SetName(d.name).SetUnique(d.unique)
		if d.filter != nil {
			opts.SetPartialFilterExpression(d.filter)
		}
		if _, err := db.Collection(d.coll).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: d.keys, Options: opts}); err != nil {
			return fmt.Errorf("create index %s.%s: %w", d.coll, d.name, err)
		}
	}
	return nil
}

// dropConflictingIndexes removes an older index that has the same key pattern
// as d but a different name or options (e.g. the plain {code:1} index created
// by the old seed), which would otherwise make creating the unique index fail.
// Only the index definition is dropped, never any documents.
func dropConflictingIndexes(ctx context.Context, coll *mongo.Collection, d idx) error {
	cur, err := coll.Indexes().List(ctx)
	if err != nil {
		if isNamespaceMissing(err) {
			return nil
		}
		return err
	}
	var existing []bson.M
	if err := cur.All(ctx, &existing); err != nil {
		return err
	}
	want := keySig(d.keys)
	for _, e := range existing {
		name, _ := e["name"].(string)
		if name == "_id_" || name == d.name {
			continue
		}
		key, ok := e["key"]
		if !ok {
			continue
		}
		kd, ok := key.(bson.D)
		if !ok {
			continue
		}
		if keySig(kd) == want {
			if _, err := coll.Indexes().DropOne(ctx, name); err != nil {
				return fmt.Errorf("drop conflicting index %s.%s: %w", coll.Name(), name, err)
			}
		}
	}
	return nil
}

func isNamespaceMissing(err error) bool {
	var ce mongo.CommandError
	return errors.As(err, &ce) && ce.Code == 26 // NamespaceNotFound
}

// keySig renders an index key pattern as "field:direction,..." so patterns
// compare equal regardless of the numeric BSON type the server stored.
func keySig(k bson.D) string {
	parts := make([]string, len(k))
	for i, e := range k {
		parts[i] = fmt.Sprintf("%s:%v", e.Key, e.Value)
	}
	return strings.Join(parts, ",")
}
