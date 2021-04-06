package pseud_uuid 

import (
	"fmt"
	"io"
	mrand "math/rand"
	"sync"
	"time"

	mi "github.com/georgercarder/mod_init"

	"github.com/pborman/uuid"
)

// random uuid from crypto/rand
func NewRandom() (id uuid.UUID) {
	id = G_UUIDInstances().newRandom()
	return
}

var g_baseReader = mrand.New(mrand.NewSource(123))

const ModInitTimeout = 4 * time.Second

// note: below is to provide pseudo-random uuids as needed by
//   many parts of this protocol. See "tunnel threads".

type UUIDFactory struct {
	sync.RWMutex
	seed               int64
	queue              UUIDQueue
	queueMin, queueMax int
}

func NewUUIDFactory(seed int64, queueMin, queueMax int) (uf *UUIDFactory) {
	uf = new(UUIDFactory)
	uf.Lock()
	defer uf.Unlock()
	isLocked := true
	uf.seed = seed
	uf.queueMin = queueMin
	uf.queueMax = queueMax
	uf.Update(isLocked)
	return
}

func (uf *UUIDFactory) Enqueue(uid uuid.UUID, isLocked bool) {
	if !isLocked {
		uf.Lock()
		defer uf.Unlock()
	}
	uf.queue.Enqueue(uid)
	return
}

func (uf *UUIDFactory) DequeueN(n int) (us []uuid.UUID) {
	uf.Lock()
	defer uf.Unlock()
	isLocked := true
	for i := 0; i < n; i++ {
		us = append(us, uf.Dequeue(isLocked))
	}
	return
}

func (uf *UUIDFactory) Dequeue(isLocked bool) (uid uuid.UUID) {
	if !isLocked {
		uf.Lock()
		isLocked = true
		defer uf.Unlock()
	}
	uid = uf.queue.Dequeue()
	uf.Update(isLocked)
	return
}

func (uf *UUIDFactory) Update(isLocked bool) {
	if !isLocked {
		uf.Lock()
		isLocked = true
		defer uf.Unlock()
	}
	if len(uf.queue) > uf.queueMin {
		return
	}
	lenQueue := len(uf.queue)
	for i := lenQueue; i < uf.queueMax; i++ {
		uid := G_UUIDInstances().GetPseudRandUUID(uf.seed)
		uf.Enqueue(uid, isLocked)
	}
	return
}

type UUIDQueue []uuid.UUID

func (q *UUIDQueue) Enqueue(uid uuid.UUID) {
	*q = append(*q, uid)
	return
}

func (q *UUIDQueue) Dequeue() (uid uuid.UUID) {
	uid = (*q)[0]
	*q = (*q)[1:]
	return
}

func G_UUIDInstances() (i *UUIDInstances) {
	ii, err := modInitializeUUID.Get()
	if err != nil {
		// TODO LogError.Println("G_UUIDInstances:", err)
		//reason := err
		//SafelyShutdown(reason)
		return
	}
	return ii.(*UUIDInstances)
}

var modInitializeUUID = mi.NewModInit(NewUUIDInstances,
	ModInitTimeout, fmt.Errorf("UUIDInstances init error."))

// uuid instances is useful in the case that there is a need
// for multiple threads of pseudorandomly generated uuids.
// The Tunnel threads of gechos relies on such threads of pseudorandomly
// generated uuids.
// See Tunnel threads for notes of why it needs this specific class
// of identifiers, or refer to whitepaper.
type UUIDInstances struct {
	// this UUIDInstances struct effectively guards the uuid package
	// which maintains an unexported "rander" rand.Reader.
	// Any call to uuid should be routed through a global UUIDInstances
	// to respect the setting of "rander" for each instance
	sync.RWMutex
	M map[int64]*UUIDInstance
}

func NewUUIDInstances() (i interface{}) { // *UUIDInstances
	ii := new(UUIDInstances)
	ii.Lock()
	defer ii.Unlock()
	ii.M = make(map[int64]*UUIDInstance)
	i = ii
	return
}

// "owner" of instance should call "defer DeleteInstance"
// to prevent memory leak
func (i *UUIDInstances) SetInstance(seed int64) {
	i.Lock()
	defer i.Unlock()
	if i.M != nil { // already set
		return
	}
	i.M[seed] = NewUUIDInstance(seed)
	return
}

// Should be called by Instance "owner" after finished to prevent
// memleak
func (i *UUIDInstances) DeleteInstance(seed int64) {
	i.Lock()
	defer i.Unlock()
	if i.M[seed] == nil {
		return
	}
	i.M[seed] = nil
	delete(i.M, seed)
	return
}

func (i *UUIDInstances) GetPseudRandUUID(seed int64) (uid uuid.UUID) {
	// seed is "base seed"
	i.Lock()
	defer i.Unlock()
	if i.M[seed] == nil {
		i.M[seed] = NewUUIDInstance(seed)
	}
	mr := i.M[seed].GetRandReader()
	uuid.SetRand(mr)
	uid = uuid.NewRandom()
	uuid.SetRand(g_baseReader)
	// to prevent uuid from reading further into mr
	return
}

// gives random from crypto/rand when pseudorandom uuid is not needed
// currently only called by simple "NewRandom" at the top of this file
// (so unexported).
func (i *UUIDInstances) newRandom() (uid uuid.UUID) {
	i.Lock()
	defer i.Unlock()
	// this is just to double check that
	// "SetRand" is set to crypto/rand
	uuid.SetRand(g_baseReader)
	uid = uuid.NewRandom()
	return
}

type UUIDInstance struct {
	r io.Reader
}

func (i *UUIDInstance) GetRandReader() (r io.Reader) {
	r = i.r
	return
}

func NewUUIDInstance(seed int64) (i *UUIDInstance) {
	i = new(UUIDInstance)
	src := mrand.NewSource(seed)
	i.r = mrand.New(src)
	return
}
