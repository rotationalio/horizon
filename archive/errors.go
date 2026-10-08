package archive

import "errors"

var (
	ErrManifestNotFound = errors.New("manifest not found in horizon archive")
	ErrClosed           = errors.New("cannot perform operation on a closed horizon archive")
	ErrDuplicateTask    = errors.New("task with ID or slug already added to manifest")
)
