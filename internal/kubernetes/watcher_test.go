// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package kubernetes

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	v1typed "k8s.io/client-go/kubernetes/typed/batch/v1"
)

// MockKubernetesClient for testing
type MockKubernetesClient struct{}

func (m *MockKubernetesClient) BatchV1() v1typed.BatchV1Interface {
	return nil // We won't use this in our simple tests
}

func TestJobWatcher_Creation(t *testing.T) {
	client := &MockKubernetesClient{}
	logger := logrus.NewEntry(logrus.New())
	
	watcher := NewJobWatcher(client, "test-namespace", logger)
	
	assert.NotNil(t, watcher)
	assert.Equal(t, "test-namespace", watcher.namespace)
	assert.NotNil(t, watcher.jobStatuses)
	assert.NotNil(t, watcher.subscribers)
}

func TestJobWatcher_SubscriptionManagement(t *testing.T) {
	client := &MockKubernetesClient{}
	logger := logrus.NewEntry(logrus.New())
	watcher := NewJobWatcher(client, "test-namespace", logger)
	
	// Test subscription
	statusCh, unsubscribe := watcher.SubscribeToJob("test-identifier")
	assert.NotNil(t, statusCh)
	assert.NotNil(t, unsubscribe)
	
	// Test unsubscribe
	unsubscribe()
	
	// Test that we can get job status (should be false initially)
	assert.False(t, watcher.IsJobFinished("test-identifier"))
}

func TestJobStatus_Structure(t *testing.T) {
	status := JobStatus{
		Identifier: "test-id",
		Finished:   true,
		Succeeded:  true,
		Failed:     false,
		LastUpdate: time.Now(),
	}
	
	assert.Equal(t, "test-id", status.Identifier)
	assert.True(t, status.Finished)
	assert.True(t, status.Succeeded)
	assert.False(t, status.Failed)
}
