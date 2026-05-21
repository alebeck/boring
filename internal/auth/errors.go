package auth

import "errors"

// ErrAborted is returned when the user cancels an auth prompt.
var ErrAborted = errors.New("authentication aborted by user")
