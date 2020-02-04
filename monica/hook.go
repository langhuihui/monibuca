package monica

var AuthHooks = make(AuthHook, 0)

type AuthHook []func(string) error

func (h AuthHook) AddHook(hook func(string) error) {
	AuthHooks = append(h, hook)
}
func (h AuthHook) Trigger(sign string) error {
	for _, f := range h {
		if err := f(sign); err != nil {
			return err
		}
	}
	return nil
}

var OnPublishHooks = make(OnPublishHook, 0)

type OnPublishHook []func(r *Room)

func (h OnPublishHook) AddHook(hook func(r *Room)) {
	OnPublishHooks = append(h, hook)
}
func (h OnPublishHook) Trigger(r *Room) {
	for _, f := range h {
		f(r)
	}
}

var OnSubscribeHooks = make(OnSubscribeHook, 0)

type OnSubscribeHook []func(s *OutputStream)

func (h OnSubscribeHook) AddHook(hook func(s *OutputStream)) {
	OnSubscribeHooks = append(h, hook)
}
func (h OnSubscribeHook) Trigger(s *OutputStream) {
	for _, f := range h {
		f(s)
	}
}

var OnDropHooks = make(OnDropHook, 0)

type OnDropHook []func(s *OutputStream)

func (h OnDropHook) AddHook(hook func(s *OutputStream)) {
	OnDropHooks = append(h, hook)
}
func (h OnDropHook) Trigger(s *OutputStream) {
	for _, f := range h {
		f(s)
	}
}

var OnSummaryHooks = make(OnSummaryHook, 0)

type OnSummaryHook []func(bool)

func (h OnSummaryHook) AddHook(hook func(bool)) {
	OnSummaryHooks = append(h, hook)
}
func (h OnSummaryHook) Trigger(v bool) {
	for _, f := range h {
		f(v)
	}
}
