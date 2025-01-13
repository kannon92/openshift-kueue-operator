// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

// UsernamePrefixApplyConfiguration represents a declarative configuration of the UsernamePrefix type for use
// with apply.
type UsernamePrefixApplyConfiguration struct {
	PrefixString *string `json:"prefixString,omitempty"`
}

// UsernamePrefixApplyConfiguration constructs a declarative configuration of the UsernamePrefix type for use with
// apply.
func UsernamePrefix() *UsernamePrefixApplyConfiguration {
	return &UsernamePrefixApplyConfiguration{}
}

// WithPrefixString sets the PrefixString field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the PrefixString field is set to the value of the last call.
func (b *UsernamePrefixApplyConfiguration) WithPrefixString(value string) *UsernamePrefixApplyConfiguration {
	b.PrefixString = &value
	return b
}
