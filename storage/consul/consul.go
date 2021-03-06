package consul

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/consul/api"
	"github.com/miracl/casper"
	"github.com/miracl/casper/consul"
	"github.com/miracl/casper/diff"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// DefaultIgnoreVal is the default value that need to be set for a key to be
// ignored.
const DefaultIgnoreVal = "_ignore"

// kv is interface that Consul KV type implements.
// Defined and used mainly for testing.
type kv interface {
	List(prefix string, q *api.QueryOptions) (api.KVPairs, *api.QueryMeta, error)
	Put(p *api.KVPair, q *api.WriteOptions) (*api.WriteMeta, error)
	Delete(key string, w *api.WriteOptions) (*api.WriteMeta, error)
}

// Storage is an implementation of the storage interface that stores in Consul KV.
type Storage struct {
	kv        kv
	ignoreVal string
}

// New returns new consul storage.
func New(addr string) (*Storage, error) {
	cfg := &api.Config{}

	ignore := ""
	if addr != "" {
		addr, err := url.Parse(addr)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing Consul address %v failed", addr)
		}
		cfg.Address = addr.Host
		cfg.Scheme = addr.Scheme
		cfg.Token = addr.Query().Get("token")

		ignore = addr.Query().Get("ignore")
	}

	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "creating Consul client failed")
	}

	if ignore == "" {
		ignore = DefaultIgnoreVal
	}
	return &Storage{client.KV(), ignore}, nil
}

func (s Storage) String(format string) (string, error) {
	pairs, _, err := s.kv.List("", nil)
	if err != nil {
		return "", err
	}
	return kvPairsToString(pairs, format), nil
}

// GetChanges returns changes between the config and the Storage content.
func (s Storage) GetChanges(config []byte, format, key string) (casper.Changes, error) {
	pairs, _, err := s.kv.List("", nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting key/value pairs from Consul failed")
	}

	return getChanges(pairs, config, format, key, s.ignoreVal)
}

// Diff returns the visual representation of the changes.
func (Storage) Diff(cs casper.Changes, pretty bool) string {
	return diff.Diff(cs.(diff.KVChanges), pretty)
}

// Push changes to the storage.
func (s Storage) Push(cs casper.Changes) error {
	for _, ci := range cs.(diff.KVChanges) {
		if err := s.push(ci); err != nil {
			return err
		}
	}
	return nil
}

func (s Storage) push(change interface{}) error {
	switch c := change.(type) {
	case *diff.Add:
		_, err := s.kv.Put(&api.KVPair{Key: c.Key(), Value: []byte(c.Val())}, nil)
		return err
	case *diff.Update:
		_, err := s.kv.Put(&api.KVPair{Key: c.Key(), Value: []byte(c.NewVal())}, nil)
		return err
	case *diff.Remove:
		_, err := s.kv.Delete(c.Key(), nil)
		return err
	}

	return fmt.Errorf("invalid change type: %T", change)
}

func kvPairsToString(pairs api.KVPairs, format string) string {
	j := consul.KVPairsToMap(pairs)

	var res []byte
	switch format {
	case "json":
		res, _ = json.MarshalIndent(j, "", "  ")
	case "jsonraw":
		res, _ = json.Marshal(j)
	default:
		res, _ = yaml.Marshal(j)

	}

	return string(res)
}

func getChanges(pairs api.KVPairs, config []byte, format, key, ignoreVal string) (diff.KVChanges, error) {
	consulChanges, err := consul.GetChanges(pairs, config, format)
	if err != nil {
		return nil, err
	}

	ignoredPaths := []string{}

	kvChanges := diff.KVChanges{}
	for _, c := range consulChanges {
		// skip ignored pairs
		if ignoreVal != "" && c.NewVal == ignoreVal {
			ignoredPaths = append(ignoredPaths, c.Key)
			continue
		}

		if key != "" && key != c.Key {
			continue
		}

		switch c.Action {
		case consul.ConsulAdd:
			kvChanges = append(kvChanges, diff.NewAdd(c.Key, c.NewVal))
		case consul.ConsulRemove:
			kvChanges = append(kvChanges, diff.NewRemove(c.Key, c.Val))
		case consul.ConsulUpdate:
			kvChanges = append(kvChanges, diff.NewUpdate(c.Key, c.Val, c.NewVal))
		}

	}

	// check for ignored folders
	notIgnoreChanges := diff.KVChanges{}
	for _, c := range kvChanges {
		if isPathIgnored(c.Key(), ignoredPaths) {
			continue
		}
		notIgnoreChanges = append(notIgnoreChanges, c)
	}

	return notIgnoreChanges, nil
}

func isPathIgnored(path string, ignoredPaths []string) bool {
	for _, p := range ignoredPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}

	return false
}
