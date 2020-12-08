package main

import "github.com/xmidt-org/bascule"

// RawAttributes is the interface that wraps methods which dictate how to interact
// with a token's attributes. It adds functionality on top of bascule Attributes by including
// a Getter for all of a token's attributes, returning a map of the attributes
type RawAttributes interface {
	bascule.Attributes
	GetRawAttributes() map[string]interface{}
}

type rawAttributes map[string]interface{}

func (r rawAttributes) Get(key string) (interface{}, bool) {
	v, ok := r[key]
	return v, ok
}

func (r rawAttributes) GetRawAttributes() map[string]interface{} {
	return r
}

// NewRawAttributes builds a RawAttributes instance with
// the given map as datasource.
func NewRawAttributes(m map[string]interface{}) RawAttributes {
	return rawAttributes(m)
}
