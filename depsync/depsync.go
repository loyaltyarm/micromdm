package depsync

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/micromdm/dep"
	"github.com/micromdm/micromdm/pubsub"
	"github.com/pkg/errors"
)

const (
	SyncTopic    = "mdm.DepSync"
	ConfigBucket = "mdm.DEPConfig"
)

type Syncer interface {
	privateDEPSyncer() bool
}

type watcher struct {
	client    dep.Client
	publisher pubsub.Publisher
	conf      *config
}

type cursor struct {
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

// A cursor is valid for a week.
func (c cursor) Valid() bool {
	expiration := time.Now().Add(24 * 7 * time.Hour)
	if c.CreatedAt.After(expiration) {
		return false
	}
	return true
}

func New(client dep.Client, pub pubsub.Publisher, db *bolt.DB) (Syncer, error) {
	conf, err := LoadConfig(db)
	if err != nil {
		return nil, err
	}
	if conf.Cursor.Valid() {
		fmt.Printf("loaded dep config with cursor: %s\n", conf.Cursor.Value)
	} else {
		conf.Cursor.Value = ""
	}
	sync := &watcher{
		publisher: pub,
		client:    client,
		conf:      conf,
	}

	saveCursor := func() {
		if err := conf.Save(); err != nil {
			log.Printf("saving cursor %s\n", err)
			return
		}
		log.Printf("saved DEP cursor at value %s\n", conf.Cursor.Value)
	}

	go func() {
		defer saveCursor()
		if err := sync.Run(); err != nil {
			log.Println("DEP watcher failed: ", err)
		}
	}()
	return sync, nil
}

// TODO this is private temporarily until the interface can be defined
func (w *watcher) privateDEPSyncer() bool {
	return true
}

// TODO this needs to be a proper error in the micromdm/dep package.
func isCursorExhausted(err error) bool {
	return strings.Contains(err.Error(), "EXHAUSTED_CURSOR")
}

func isCursorExpired(err error) bool {
	return strings.Contains(err.Error(), "EXPIRED_CURSOR")
}

func (w *watcher) Run() error {
	ticker := time.NewTicker(30 * time.Minute).C
FETCH:
	for {
		resp, err := w.client.FetchDevices(dep.Limit(100), dep.Cursor(w.conf.Cursor.Value))
		if err != nil && isCursorExhausted(err) {
			goto SYNC
		} else if err != nil {
			return err
		}
		fmt.Printf("more=%v, cursor=%s, fetched=%v\n", resp.MoreToFollow, resp.Cursor, resp.FetchedUntil)
		w.conf.Cursor = cursor{Value: resp.Cursor, CreatedAt: time.Now()}
		if err := w.conf.Save(); err != nil {
			return errors.Wrap(err, "saving cursor from fetch")
		}
		e := NewEvent(resp.Devices)
		data, err := MarshalEvent(e)
		if err != nil {
			return err
		}
		if err := w.publisher.Publish(SyncTopic, data); err != nil {
			return err
		}
		if !resp.MoreToFollow {
			goto SYNC
		}
	}

SYNC:
	for {
		resp, err := w.client.SyncDevices(w.conf.Cursor.Value, dep.Cursor(w.conf.Cursor.Value))
		if err != nil && isCursorExpired(err) {
			w.conf.Cursor.Value = ""
			goto FETCH
		} else if err != nil {
			return err
		}
		if len(resp.Devices) != 0 {
			fmt.Printf("more=%v, cursor=%s, synced=%v\n", resp.MoreToFollow, resp.Cursor, resp.FetchedUntil)
		}
		w.conf.Cursor = cursor{Value: resp.Cursor, CreatedAt: time.Now()}
		if err := w.conf.Save(); err != nil {
			return errors.Wrap(err, "saving cursor from sync")
		}

		// TODO handle sync response here.
		<-ticker
	}
}
