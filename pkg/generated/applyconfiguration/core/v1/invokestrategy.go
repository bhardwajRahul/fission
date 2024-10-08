/*
Copyright The Fission Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

import (
	corev1 "github.com/fission/fission/pkg/apis/core/v1"
)

// InvokeStrategyApplyConfiguration represents a declarative configuration of the InvokeStrategy type for use
// with apply.
type InvokeStrategyApplyConfiguration struct {
	ExecutionStrategy *ExecutionStrategyApplyConfiguration `json:"ExecutionStrategy,omitempty"`
	StrategyType      *corev1.StrategyType                 `json:"StrategyType,omitempty"`
}

// InvokeStrategyApplyConfiguration constructs a declarative configuration of the InvokeStrategy type for use with
// apply.
func InvokeStrategy() *InvokeStrategyApplyConfiguration {
	return &InvokeStrategyApplyConfiguration{}
}

// WithExecutionStrategy sets the ExecutionStrategy field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ExecutionStrategy field is set to the value of the last call.
func (b *InvokeStrategyApplyConfiguration) WithExecutionStrategy(value *ExecutionStrategyApplyConfiguration) *InvokeStrategyApplyConfiguration {
	b.ExecutionStrategy = value
	return b
}

// WithStrategyType sets the StrategyType field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the StrategyType field is set to the value of the last call.
func (b *InvokeStrategyApplyConfiguration) WithStrategyType(value corev1.StrategyType) *InvokeStrategyApplyConfiguration {
	b.StrategyType = &value
	return b
}
