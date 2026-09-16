package schema

type Validator interface {
	Validate(data any) error
}
