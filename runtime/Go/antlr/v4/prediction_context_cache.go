package antlr

import "sync"

// PredictionContextCache is Used to cache [PredictionContext] objects. It is used for the shared
// context cash associated with contexts in DFA states. This cache
// can be used for both lexers and parsers.
type PredictionContextCache struct {
	singletons map[uint64]ContextID
	arrays     map[int][]ContextID
	lock       sync.RWMutex
}

func NewPredictionContextCache() *PredictionContextCache {
	return &PredictionContextCache{
		singletons: make(map[uint64]ContextID),
		arrays:     make(map[int][]ContextID),
	}
}

// Add a context to the cache and return it. If the context already exists,
// return that one instead and do not add a new context to the cache.
// Protect shared cache from unsafe thread access.
func (p *PredictionContextCache) add(ctx *PredictionContext) *PredictionContext {
	if ctx.isEmpty() {
		return BasePredictionContextEMPTY
	}

	if ctx.pcType == PredictionContextSingleton {
		key := uint64(ctx.parentID)<<32 | uint64(uint32(ctx.returnState))
		p.lock.RLock()
		id, ok := p.singletons[key]
		if ok {
			p.lock.RUnlock()
			if collectStats {
				Statistics.AddContextCacheHit()
			}
			return getContextByID(id)
		}
		p.lock.RUnlock()

		p.lock.Lock()
		defer p.lock.Unlock()
		if id, ok := p.singletons[key]; ok {
			return getContextByID(id)
		}
		nextID(ctx)
		p.singletons[key] = ctx.id
		return ctx
	}

	h := ctx.Hash()
	p.lock.RLock()
	ids := p.arrays[h]
	for _, id := range ids {
		existing := getContextByID(id)
		if existing.Equals(ctx) {
			p.lock.RUnlock()
			if collectStats {
				Statistics.AddContextCacheHit()
			}
			return existing
		}
	}
	p.lock.RUnlock()

	p.lock.Lock()
	defer p.lock.Unlock()

	// Re-check after acquiring write lock
	ids = p.arrays[h]
	for _, id := range ids {
		existing := getContextByID(id)
		if existing.Equals(ctx) {
			return existing
		}
	}

	nextID(ctx)
	p.arrays[h] = append(ids, ctx.id)
	return ctx
}

func (p *PredictionContextCache) Get(ctx *PredictionContext) (*PredictionContext, bool) {
	if ctx.isEmpty() {
		return BasePredictionContextEMPTY, true
	}

	if ctx.pcType == PredictionContextSingleton {
		key := uint64(ctx.parentID)<<32 | uint64(uint32(ctx.returnState))
		p.lock.RLock()
		defer p.lock.RUnlock()
		id, ok := p.singletons[key]
		if ok {
			return getContextByID(id), true
		}
		return nil, false
	}

	h := ctx.Hash()
	p.lock.RLock()
	defer p.lock.RUnlock()
	ids, present := p.arrays[h]
	if present {
		for _, id := range ids {
			existing := getContextByID(id)
			if existing.Equals(ctx) {
				return existing, true
			}
		}
	}
	return nil, false
}

func (p *PredictionContextCache) length() int {
	p.lock.RLock()
	defer p.lock.RUnlock()
	l := len(p.singletons)
	for _, ids := range p.arrays {
		l += len(ids)
	}
	return l
}
