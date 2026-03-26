package gitstore

import "errors"

// ErrNotRegistered is returned when the requested repo is not in the config.
var ErrNotRegistered = errors.New("repo not registered")

// ErrNotFound is returned when a path does not exist in the repo tree.
var ErrNotFound = errors.New("not found")
