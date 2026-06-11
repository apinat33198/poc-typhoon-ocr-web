package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"time"
)

type Doc struct {
	ID    string
	Path  string
	Name  string
	Pages int
	IsPDF bool
	TS    time.Time
}

// Store keeps uploaded documents in temp files and deletes them after a TTL.
type Store struct {
	mu   sync.Mutex
	docs map[string]*Doc
	ttl  time.Duration
}

func NewStore(ttl time.Duration) *Store {
	s := &Store{docs: make(map[string]*Doc), ttl: ttl}
	go s.janitor()
	return s
}

func (s *Store) Add(path, name string, pages int, isPDF bool) *Doc {
	b := make([]byte, 6)
	rand.Read(b)
	doc := &Doc{
		ID: hex.EncodeToString(b), Path: path, Name: name,
		Pages: pages, IsPDF: isPDF, TS: time.Now(),
	}
	s.mu.Lock()
	s.docs[doc.ID] = doc
	s.mu.Unlock()
	return doc
}

func (s *Store) Get(id string) (*Doc, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.docs[id]
	if ok {
		if _, err := os.Stat(doc.Path); err != nil {
			delete(s.docs, id)
			return nil, false
		}
	}
	return doc, ok
}

func (s *Store) janitor() {
	for range time.Tick(5 * time.Minute) {
		now := time.Now()
		s.mu.Lock()
		for id, doc := range s.docs {
			if now.Sub(doc.TS) > s.ttl {
				os.Remove(doc.Path)
				delete(s.docs, id)
			}
		}
		s.mu.Unlock()
	}
}
