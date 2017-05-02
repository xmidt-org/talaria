package main

import (
	"github.com/stretchr/testify/mock"
)

type mockURLFilter struct {
	mock.Mock
}

func (m *mockURLFilter) Filter(v string) (string, error) {
	arguments := m.Called(v)
	return arguments.String(0), arguments.Error(1)
}
