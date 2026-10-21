package store

import (
	"encoding/json"
	"os"
	"time"

	"github.com/Jatin-dudhani/my-redis/ds"
)

type StoredValue struct {
	Type      string            `json:"type"` // "string", "list", "set", "hash", "zset"
	Str       string            `json:"str,omitempty"`
	List      []string          `json:"list,omitempty"`
	Set       []string          `json:"set,omitempty"`
	Hash      map[string]string `json:"hash,omitempty"`
	ZSet      []ZSetJSONEntry   `json:"zset,omitempty"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
}

type ZSetJSONEntry struct {
	Member string  `json:"member"`
	Score  float64 `json:"score"`
}

func SaveToFile(s *Store, path string) error {
	dumps := s.DumpAll()
	persisted := make(map[string]StoredValue, len(dumps))

	for _, d := range dumps {
		sv := StoredValue{ExpiresAt: d.ExpiresAt}
		switch val := d.Value.(type) {
		case string:
			sv.Type = "string"
			sv.Str = val
		case *ds.List:
			sv.Type = "list"
			sv.List = val.Items()
		case *ds.Set:
			sv.Type = "set"
			sv.Set = val.SMembers()
		case *ds.Hash:
			sv.Type = "hash"
			sv.Hash = val.HGetAll()
		case *ds.SSet:
			sv.Type = "zset"
			entries := val.Entries()
			zentries := make([]ZSetJSONEntry, len(entries))
			for i, e := range entries {
				zentries[i] = ZSetJSONEntry{Member: e.Member, Score: e.Score}
			}
			sv.ZSet = zentries
		default:
			continue
		}
		persisted[d.Key] = sv
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(persisted)
}

func LoadFromFile(path string) (*Store, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Try reading structured StoredValue format
	var complexData map[string]StoredValue
	decoder := json.NewDecoder(f)
	err = decoder.Decode(&complexData)
	if err == nil {
		s := New()
		now := time.Now()
		isStructured := true
		for _, v := range complexData {
			if v.Type == "" {
				isStructured = false
				break
			}
		}

		if isStructured {
			for k, sv := range complexData {
				if sv.ExpiresAt != nil && now.After(*sv.ExpiresAt) {
					continue
				}
				var val interface{}
				switch sv.Type {
				case "string":
					val = sv.Str
				case "list":
					l := ds.NewList()
					l.RPush(sv.List...)
					val = l
				case "set":
					st := ds.NewSet()
					st.SAdd(sv.Set...)
					val = st
				case "hash":
					h := ds.NewHash()
					for fld, fv := range sv.Hash {
						h.HSet(fld, fv)
					}
					val = h
				case "zset":
					zs := ds.NewSSet()
					for _, ze := range sv.ZSet {
						zs.ZAdd(ze.Score, ze.Member)
					}
					val = zs
				default:
					continue
				}

				if sv.ExpiresAt != nil {
					ttl := sv.ExpiresAt.Sub(now)
					s.SetWithTTL(k, val, ttl)
				} else {
					s.Set(k, val)
				}
			}
			return s, nil
		}
	}

	// Fallback to legacy string map
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	var legacyData map[string]string
	if err := json.NewDecoder(f).Decode(&legacyData); err != nil {
		return nil, err
	}
	s := New()
	s.LoadStrings(legacyData)
	return s, nil
}
