package cache

import (
	"errors"
	"reflect"
	"sync"
)

// ，
// SimpleCache is a simple cache implementation
type SimpleCache struct {
	data      map[string]interface{}
	mu        sync.Mutex
	maxBytes  int64
	usedBytes int64
}

func (s *SimpleCache) Get(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Ptr {
		return errors.New("value must be a pointer")
	}
	v, ok := s.data[key]
	if !ok {

		val.Elem().Set(reflect.Zero(val.Elem().Type()))
		return nil
	}

	vval := reflect.ValueOf(v)
	if val.Elem().Type() != vval.Type() {

		val.Elem().Set(reflect.Zero(val.Elem().Type()))
		return nil
	}

	val.Elem().Set(reflect.ValueOf(v))
	return nil
}

func (s *SimpleCache) Set(key string, value interface{}, exp int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// ，
	val := reflect.ValueOf(value)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	s.data[key] = val.Interface()
	return nil
}
func (s *SimpleCache) Gc() error {
	return nil
}

func NewSimpleCache() *SimpleCache {
	return &SimpleCache{
		data: make(map[string]interface{}),
	}
}
