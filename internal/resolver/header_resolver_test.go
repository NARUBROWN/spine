package resolver

import (
	"errors"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/header"
)

// MockRequestContext is a mock of RequestContext for testing purpose
type MockRequestContext struct {
	core.RequestContext
	headers map[string][]string
}

// Headers is an implementation of RequestContext interface that returns mock headers
func (m *MockRequestContext) Headers() map[string][]string {
	return m.headers
}

func TestHeaderResolver_Supports(t *testing.T) {
	testCases := []struct {
		name           string
		pm             ParameterMeta
		expectedResult bool
	}{
		{
			name:           "HeaderResolver should support header type",
			pm:             ParameterMeta{Type: reflect.TypeOf(header.Values{})},
			expectedResult: true,
		},
		{
			name:           "HeaderResolver should not support other datatype",
			pm:             ParameterMeta{Type: reflect.TypeOf("test")},
			expectedResult: false,
		},
	}
	for _, testCase := range testCases {
		resolver := HeaderResolver{}
		actualResult := resolver.Supports(testCase.pm)
		if actualResult != testCase.expectedResult {
			t.Errorf("HeaderResolver.Supports() should return %v for %s", testCase.expectedResult, testCase.name)
		}
	}
}

func TestHeaderResolver_Resolve(t *testing.T) {
	testCases := []struct {
		name            string
		expectedHeaders map[string][]string
		expectedError   error
	}{
		{
			name: "HeaderResolver should resolve current headers",
			expectedHeaders: map[string][]string{
				"Host":       {"localhost:8080"},
				"User-Agent": {"curl/7.64.1"},
				"Accept":     {"application/json"},
			},
			expectedError: nil,
		},
		{
			name:            "HeaderResolver should return error when headers is nil",
			expectedHeaders: nil,
			expectedError:   nil,
		},
		{
			name:            "HeaderResolver should return error when headers is empty",
			expectedHeaders: map[string][]string{},
			expectedError:   nil,
		},
	}
	for _, testCase := range testCases {
		resolver := HeaderResolver{}
		ctx := &MockRequestContext{headers: testCase.expectedHeaders}
		actualResult, err := resolver.Resolve(ctx, ParameterMeta{Type: reflect.TypeOf(header.Values{})})

		if !errors.Is(err, testCase.expectedError) {
			t.Errorf("HeaderResolver.Resolve() should return error %v for %s", testCase.expectedError, testCase.name)
		}

		expectedResult := header.NewValues(testCase.expectedHeaders)
		if !reflect.DeepEqual(actualResult, expectedResult) {
			t.Errorf("HeaderResolver.Resolve() should return %v but got %v", expectedResult, actualResult)
		}

	}
}
