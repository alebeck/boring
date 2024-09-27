package tunnel

import (
	"fmt"
	"strconv"
)

// Custom type to handle both string and integer values
// in the TOML config. This is useful for the local address.
type StringOrInt string

func (s *StringOrInt) UnmarshalTOML(v any) error {
	switch value := v.(type) {
	case int64:
		*s = StringOrInt(strconv.FormatInt(value, 10)) // convert integer to string
	case string:
		*s = StringOrInt(value)
	default:
		return fmt.Errorf("unsupported type: %T", v)
	}
	return nil
}

func (s StringOrInt) String() string {
	return string(s)
}
