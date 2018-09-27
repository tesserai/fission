package storagesvc

import (
	"fmt"
	"io"
	"sync"

	"github.com/fission/fission/storagesvc/progress"
)

type (
	pendingUpload struct {
		reader *progress.Reader
		size   int64
	}
	UploadRegistry struct {
		mutex   sync.RWMutex
		pending map[string]*pendingUpload
	}
)

func NewUploadRegistry() *UploadRegistry {
	return &UploadRegistry{
		pending: map[string]*pendingUpload{},
	}
}

func (reg *UploadRegistry) declare(uploadName string, size int64, reader io.Reader) *progress.Reader {
	r := progress.NewReader(reader)
	reg.mutex.Lock()
	defer reg.mutex.Unlock()

	fmt.Printf("declare(%s, ...)\n", uploadName)
	reg.pending[uploadName] = &pendingUpload{reader: r, size: size}
	return r
}

func (reg *UploadRegistry) get(uploadName string) (progress.Counter, int64) {
	reg.mutex.RLock()
	defer reg.mutex.RUnlock()

	fmt.Printf("get(%s, ...)\n", uploadName)
	pending, ok := reg.pending[uploadName]
	if !ok {
		return nil, -1
	}
	return pending.reader, pending.size
}

func (reg *UploadRegistry) remove(uploadName string, r *progress.Reader) {
	reg.mutex.Lock()
	defer reg.mutex.Unlock()

	fmt.Printf("remove(%s, ...)\n", uploadName)
	existing, ok := reg.pending[uploadName]
	if ok && existing.reader == r {
		delete(reg.pending, uploadName)
	}
}
