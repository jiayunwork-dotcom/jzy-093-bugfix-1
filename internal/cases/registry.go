// Package cases 负责具名工况档的登记与取用（registry），
// 并在初始化时内置一份小型苦咸水淡化工况档。
//
// Registry 不缓存任何计算结果：每次 Evaluate 都拿到一份规格副本再独立计算，
// 因此同一进程里两份不同工况档的结果互不串名、互不影响。
package cases

import (
	"fmt"
	"sort"
	"sync"

	"rocalc/internal/membrane"
	"rocalc/internal/validation"
)

// Spec 是对外登记的工况档：一个具名的膜核算输入。
type Spec struct {
	Name string
	membrane.FeedSpec
}

// Registry 是线程安全的具名工况档登记表。
type Registry struct {
	mu    sync.RWMutex
	specs map[string]Spec
}

// NewRegistry 创建空登记表。
func NewRegistry() *Registry {
	return &Registry{specs: make(map[string]Spec)}
}

var (
	// ErrAlreadyExists 重复登记同名工况档时返回。
	ErrAlreadyExists = fmt.Errorf("case already exists")
	// ErrNotFound 点名了不存在的工况档时返回。
	ErrNotFound = fmt.Errorf("case not found")
)

// Register 登记一份新工况档；同名已存在返回 ErrAlreadyExists。
func (r *Registry) Register(spec Spec) error {
	if spec.Name == "" {
		return validation.NewError(validation.CodeNonPositiveParameter, "工况档名称不能为空")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.specs[spec.Name]; ok {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, spec.Name)
	}
	r.specs[spec.Name] = cloneSpec(spec)
	return nil
}

// Put 登记或整体替换一份工况档（upsert 语义，对应 HTTP PUT）。
func (r *Registry) Put(spec Spec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs[spec.Name] = cloneSpec(spec)
}

// Get 取一份工况档的副本。副本与登记表内部数据互不共享，
// 调用方的任何改动都不会污染已登记工况档。
func (r *Registry) Get(name string) (Spec, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.specs[name]
	if !ok {
		return Spec{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return cloneSpec(spec), nil
}

// List 返回按名字排序的已登记工况档名称。
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.specs))
	for name := range r.specs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Delete 删除一份工况档；不存在返回 ErrNotFound。
func (r *Registry) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.specs[name]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	delete(r.specs, name)
	return nil
}

// Evaluate 点名一份工况档，取其副本独立完成一次膜核算。
func (r *Registry) Evaluate(name string) (*membrane.Outcome, error) {
	spec, err := r.Get(name)
	if err != nil {
		return nil, err
	}
	return membrane.Evaluate(spec.FeedSpec)
}

func cloneSpec(s Spec) Spec {
	cp := s
	// Solution 目前全是标量字段，值拷贝即可；保留函数便于以后扩展。
	return cp
}
