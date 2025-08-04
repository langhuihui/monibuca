package task

type Work struct {
	Job
}

func (m *Work) keepalive() bool {
	return true
}

func (*Work) GetTaskType() TaskType {
	return TASK_TYPE_Work
}

type WorkCollection[K comparable, T interface {
	ITask
	GetKey() K
}] struct {
	Work
}

func (c *WorkCollection[K, T]) Find(f func(T) bool) (item T, ok bool) {
	c.RangeSubTask(func(task ITask) bool {
		if v, _ok := task.(T); _ok && f(v) {
			item = v
			ok = true
			return false
		}
		return true
	})
	return
}

func (c *WorkCollection[K, T]) Get(key K) (item T, ok bool) {
	var value any
	value, ok = c.children.Load(key)
	if ok {
		item, ok = value.(T)
	}
	return
}

func (c *WorkCollection[K, T]) Range(f func(T) bool) {
	c.RangeSubTask(func(task ITask) bool {
		if v, ok := task.(T); ok && !f(v) {
			return false
		}
		return true
	})
}

func (c *WorkCollection[K, T]) Has(key K) (ok bool) {
	_, ok = c.children.Load(key)
	return
}

func (c *WorkCollection[K, T]) ToList() (list []T) {
	c.Range(func(t T) bool {
		list = append(list, t)
		return true
	})
	return
}

func (c *WorkCollection[K, T]) Length() int {
	return int(c.Size.Load())
}
