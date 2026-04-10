package module

// Module is implemented by any component that requires one-time initialization.
type Module interface {
	Apply() error
}

// Apply calls Apply on each module in order, stopping on the first error.
func Apply(m ...Module) error {
	for _, mod := range m {
		if err := mod.Apply(); err != nil {
			return err
		}
	}
	return nil
}
