package errs

import "errors"

// User is a message that can be shown to the person using Phlo.
type User struct {
	Message string
}

func (e *User) Error() string { return e.Message }

func New(message string) error { return &User{Message: message} }

func As(err error) (*User, bool) {
	var user *User
	if errors.As(err, &user) {
		return user, true
	}
	return nil, false
}
