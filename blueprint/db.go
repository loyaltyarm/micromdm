package blueprint

import (
	"fmt"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/pkg/errors"

	"github.com/micromdm/micromdm/profile"
)

const (
	BlueprintBucket      = "mdm.Blueprint"
	blueprintIndexBucket = "mdm.BlueprintIdx"
)

type DB struct {
	*bolt.DB
	profDB *profile.DB
}

func NewDB(db *bolt.DB, pDB *profile.DB) (*DB, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(blueprintIndexBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(BlueprintBucket))
		return err
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating %s bucket", BlueprintBucket)
	}
	datastore := &DB{
		DB:     db,
		profDB: pDB,
	}
	return datastore, nil
}

func (db *DB) Create(bp *Blueprint) error {
	_, err := db.BlueprintByName(bp.Name)
	if err != nil && isNotFound(err) {
		return errors.New("blueprint must have a unique name")
	} else if err != nil {
		return errors.Wrap(err, "creating blueprint")
	}

	return db.Save(bp)
}

func (db *DB) List() ([]Blueprint, error) {
	// TODO add filter/limit with ForEach
	var blueprints []Blueprint
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlueprintBucket))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var bp Blueprint
			if err := UnmarshalBlueprint(v, &bp); err != nil {
				return err
			}
			blueprints = append(blueprints, bp)
		}
		return nil
	})
	return blueprints, err
}

func (db *DB) Save(bp *Blueprint) error {
	err := bp.Verify()
	if err != nil {
		return err
	}
	// verify that each Profile ID represents a profile we know about
	for _, p := range bp.ProfileIdentifiers {
		if _, err := db.profDB.ProfileById(p); err != nil {
			if profile.IsNotFound(err) {
				return errors.New(fmt.Sprintf("Profile ID %s in Blueprint %s does not exist", p, bp.Name))
			}
			return errors.Wrap(err, "fetching profile")
		}
	}
	tx, err := db.DB.Begin(true)
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}
	bkt := tx.Bucket([]byte(BlueprintBucket))
	if bkt == nil {
		return fmt.Errorf("bucket %q not found!", BlueprintBucket)
	}
	bpproto, err := MarshalBlueprint(bp)
	if err != nil {
		return errors.Wrap(err, "marshalling blueprint")
	}
	indexes := []string{bp.UUID, bp.Name}
	idxBucket := tx.Bucket([]byte(blueprintIndexBucket))
	if idxBucket == nil {
		return fmt.Errorf("bucket %q not found!", idxBucket)
	}
	for _, idx := range indexes {
		if idx == "" {
			continue
		}
		key := []byte(idx)
		if err := idxBucket.Put(key, []byte(bp.UUID)); err != nil {
			return errors.Wrap(err, "put blueprint idx to boltdb")
		}
	}

	key := []byte(bp.UUID)
	if err := bkt.Put(key, bpproto); err != nil {
		return errors.Wrap(err, "put blueprint to boltdb")
	}
	return tx.Commit()
}

func (db *DB) BlueprintByName(name string) (*Blueprint, error) {
	var bp Blueprint
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlueprintBucket))
		ib := tx.Bucket([]byte(blueprintIndexBucket))
		idx := ib.Get([]byte(name))
		if idx == nil {
			return &notFound{"Blueprint", fmt.Sprintf("name %s", name)}
		}
		v := b.Get(idx)
		if idx == nil {
			return &notFound{"Blueprint", fmt.Sprintf("uuid %s", string(idx))}
		}
		return UnmarshalBlueprint(v, &bp)
	})
	if err != nil {
		return nil, err
	}
	return &bp, nil
}

func (db *DB) BlueprintsByApplyAt(name string) ([]*Blueprint, error) {
	var bps []*Blueprint
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlueprintBucket))
		c := b.Cursor()
		// TODO: fix this to use an index of ApplyAt strings mapping to
		// an array of Blueprints or other more efficient means. Looping
		// over every blueprint is quite inefficient!
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var bp Blueprint
			err := UnmarshalBlueprint(v, &bp)
			if err != nil {
				fmt.Println("could not Unmarshal Blueprint")
				continue
			}
			for _, n := range bp.ApplyAt {
				if strings.ToLower(n) == strings.ToLower(name) {
					bps = append(bps, &bp)
					break
				}
			}
		}
		return nil
	})
	return bps, err
}

type notFound struct {
	ResourceType string
	Message      string
}

func (e *notFound) Error() string {
	return fmt.Sprintf("not found: %s %s", e.ResourceType, e.Message)
}

func isNotFound(err error) bool {
	if _, ok := err.(*notFound); ok {
		return true
	}
	return false
}
