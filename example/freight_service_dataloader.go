package example

import (
	"context"
	"fmt"
	"sync"
	"time"

	freightv1 "go.einride.tech/protoc-gen-go-aip-dataloader/example/internal/proto/gen/einride/example/freight/v1"
	"google.golang.org/protobuf/proto"
)

// SitesDataloader is a dataloader for einride.example.freight.v1.FreightService.BatchGetSites.
type SitesDataloader struct {
	ctx      context.Context
	client   freightv1.FreightServiceClient
	wait     time.Duration
	maxBatch int
	mu       sync.Mutex // protects mutable state below
	cache    map[string]*freightv1.Site
	batches  map[string]*sitesDataloaderBatch
}

type sitesDataloaderBatch struct {
	parent  string
	keys    []string
	data    []*freightv1.Site
	err     error
	closing bool
	done    chan struct{}
}

// NewSitesDataloader creates a new dataloader for einride.example.freight.v1.FreightService.BatchGetSites.
func NewSitesDataloader(
	ctx context.Context,
	client freightv1.FreightServiceClient,
	wait time.Duration,
	maxBatch int,
) *SitesDataloader {
	return &SitesDataloader{
		ctx:      ctx,
		client:   client,
		wait:     wait,
		maxBatch: maxBatch,
	}
}

func (l *SitesDataloader) fetch(parent string, keys []string) ([]*freightv1.Site, error) {
	var request freightv1.BatchGetSitesRequest
	// request := proto.Clone(l.requestTemplate).(*freightv1.BatchGetSitesRequest)
	request.Parent = parent
	request.Names = keys
	response, err := l.client.BatchGetSites(l.ctx, &request)
	if err != nil {
		return nil, err
	}
	return response.Sites, nil
}

// Load a result by key, batching and caching will be applied automatically.
func (l *SitesDataloader) Load(parent, name string) (*freightv1.Site, error) {
	if l == nil {
		return nil, fmt.Errorf("SitesDataloader is nil")
	}
	return l.LoadThunk(parent, name)()
}

// LoadThunk returns a function that when called will block waiting for a result.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *SitesDataloader) LoadThunk(parent, name string) func() (*freightv1.Site, error) {
	if l == nil {
		return func() (*freightv1.Site, error) {
			return nil, fmt.Errorf("SitesDataloader is nil")
		}
	}
	l.mu.Lock()
	key := name
	if it, ok := l.cache[key]; ok {
		l.mu.Unlock()
		return func() (*freightv1.Site, error) {
			return it, nil
		}
	}
	if l.batches == nil {
		l.batches = make(map[string]*sitesDataloaderBatch)
	}
	batch, ok := l.batches[parent]
	if !ok {
		batch = &sitesDataloaderBatch{parent: parent, done: make(chan struct{})}
		l.batches[parent] = batch
	}
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()
	return func() (*freightv1.Site, error) {
		<-batch.done
		var data *freightv1.Site
		if pos < len(batch.data) {
			data = batch.data[pos]
		}
		if batch.err == nil {
			l.mu.Lock()
			l.unsafeSet(key, data)
			l.mu.Unlock()
		}
		return data, batch.err
	}
}

// LoadAll fetches many keys at once.
// It will be broken into appropriately sized sub-batches based on how the dataloader is configured.
func (l *SitesDataloader) LoadAll(parent string, names []string) ([]*freightv1.Site, error) {
	keys := names
	results := make([]func() (*freightv1.Site, error), len(keys))
	for i, key := range keys {
		results[i] = l.LoadThunk(parent, key)
	}
	values := make([]*freightv1.Site, len(keys))
	var err error
	for i, thunk := range results {
		values[i], err = thunk()
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

// LoadAllThunk returns a function that when called will block waiting for results.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *SitesDataloader) LoadAllThunk(parent string, names []string) func() ([]*freightv1.Site, error) {
	keys := names
	results := make([]func() (*freightv1.Site, error), len(keys))
	for i, key := range keys {
		results[i] = l.LoadThunk(parent, key)
	}
	return func() ([]*freightv1.Site, error) {
		values := make([]*freightv1.Site, len(keys))
		var err error
		for i, thunk := range results {
			values[i], err = thunk()
			if err != nil {
				return nil, err
			}
		}
		return values, nil
	}
}

// Prime the cache with the provided key and value. If the key already exists, no change is made
// and false is returned.
// (To forcefully prime the cache, clear the key first with loader.clear(key).prime(key, value).)
func (l *SitesDataloader) Prime(key string, value *freightv1.Site) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	var found bool
	if _, found = l.cache[key]; !found {
		// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
		// and end up with the whole cache pointing to the same value.
		l.unsafeSet(key, proto.Clone(value).(*freightv1.Site))
	}
	l.mu.Unlock()
	return !found
}

// Clear the value at key from the cache, if it exists.
func (l *SitesDataloader) Clear(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.cache, key)
	l.mu.Unlock()
}

func (l *SitesDataloader) unsafeSet(key string, value *freightv1.Site) {
	if l.cache == nil {
		l.cache = map[string]*freightv1.Site{}
	}
	l.cache[key] = value
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch.
func (b *sitesDataloaderBatch) keyIndex(l *SitesDataloader, key string) int {
	for i, existingKey := range b.keys {
		if key == existingKey {
			return i
		}
	}
	pos := len(b.keys)
	b.keys = append(b.keys, key)
	if pos == 0 {
		go b.startTimer(l)
	}
	if l.maxBatch != 0 && pos >= l.maxBatch-1 {
		if !b.closing {
			b.closing = true
			delete(l.batches, b.parent)
			go b.end(l)
		}
	}
	return pos
}

func (b *sitesDataloaderBatch) startTimer(l *SitesDataloader) {
	time.Sleep(l.wait)
	l.mu.Lock()
	// we must have hit a batch limit and are already finalizing this batch
	if b.closing {
		l.mu.Unlock()
		return
	}
	delete(l.batches, b.parent)
	l.mu.Unlock()
	b.end(l)
}

func (b *sitesDataloaderBatch) end(l *SitesDataloader) {
	b.data, b.err = l.fetch(b.parent, b.keys)
	close(b.done)
}
