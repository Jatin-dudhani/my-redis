package store

import (
	"encoding/json"
	"os"
)

func SaveToFile(s *Store, path string) error {
	data := s.AllStrings()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func LoadFromFile(path string) (*Store, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var data map[string]string
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, err
	}
	s := New()
	s.LoadStrings(data)
	return s, nil
}
