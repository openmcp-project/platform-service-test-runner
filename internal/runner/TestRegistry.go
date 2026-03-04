package runner

type TestRegistry struct {
	registry map[string]TestCase
}

func NewTestRegistry() *TestRegistry {
	return &TestRegistry{
		registry: make(map[string]TestCase),
	}
}

func (tr *TestRegistry) RegisterTestCase(name string, tc TestCase) {
	tr.registry[name] = tc
}

func (tr *TestRegistry) GetTestCase(name string) (TestCase, bool) {
	tc, exists := tr.registry[name]
	return tc, exists
}
