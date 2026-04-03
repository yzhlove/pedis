package modules

type Modules interface {
	Apply() error
}

func Apply(m ...Modules) error {
	for _, mod := range m {
		if err := mod.Apply(); err != nil {
			return err
		}
	}
	return nil
}
