// Code generated by mockery v1.0.0. DO NOT EDIT.

package rfclient

import (
	context "context"

	routing "github.com/skycoin/skywire/pkg/routing"
	mock "github.com/stretchr/testify/mock"
)

// MockClient is an autogenerated mock type for the Client type
type MockClient struct {
	mock.Mock
}

// FindRoutes provides a mock function with given fields: ctx, rts, opts
func (_m *MockClient) FindRoutes(ctx context.Context, rts []routing.PathEdges, opts *RouteOptions) (map[routing.PathEdges][][]routing.Hop, error) {
	ret := _m.Called(ctx, rts, opts)

	var r0 map[routing.PathEdges][][]routing.Hop
	if rf, ok := ret.Get(0).(func(context.Context, []routing.PathEdges, *RouteOptions) map[routing.PathEdges][][]routing.Hop); ok {
		r0 = rf(ctx, rts, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[routing.PathEdges][][]routing.Hop)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, []routing.PathEdges, *RouteOptions) error); ok {
		r1 = rf(ctx, rts, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Health provides a mock function with given fields: ctx
func (_m *MockClient) Health(ctx context.Context) (int, error) {
	ret := _m.Called(ctx)

	var r0 int
	if rf, ok := ret.Get(0).(func(context.Context) int); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(int)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
