package runner

// TestRegistry holds a mapping of test case names to their implementations.
type TestRegistry struct {
	registry map[string]TestCase
}

// NewTestRegistry creates and returns a new empty TestRegistry.
func NewTestRegistry() *TestRegistry {
	return &TestRegistry{
		registry: make(map[string]TestCase),
	}
}

// RegisterTestCase registers a test case implementation under the given name.
func (tr *TestRegistry) RegisterTestCase(name string, tc TestCase) {
	tr.registry[name] = tc
}

// GetTestCase retrieves a registered test case by name. Returns false if not found.
func (tr *TestRegistry) GetTestCase(name string) (TestCase, bool) {
	tc, exists := tr.registry[name]
	return tc, exists
}
