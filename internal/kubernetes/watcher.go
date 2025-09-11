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
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	v1typed "k8s.io/client-go/kubernetes/typed/batch/v1"
)

// KubernetesClient defines the interface we need from the Kubernetes client
type KubernetesClient interface {
	BatchV1() v1typed.BatchV1Interface
}

// JobStatus represents the status of a job
type JobStatus struct {
	Identifier string
	Finished   bool
	Succeeded  bool
	Failed     bool
	LastUpdate time.Time
}

// JobWatcher manages watching Kubernetes jobs for status changes
type JobWatcher struct {
	logger         *logrus.Entry
	client         KubernetesClient
	namespace      string
	jobStatuses    map[string]*JobStatus
	statusMutex    sync.RWMutex
	subscribers    map[string][]chan<- JobStatus
	subMutex       sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// NewJobWatcher creates a new job watcher
func NewJobWatcher(client KubernetesClient, namespace string, logger *logrus.Entry) *JobWatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &JobWatcher{
		logger:      logger,
		client:      client,
		namespace:   namespace,
		jobStatuses: make(map[string]*JobStatus),
		subscribers: make(map[string][]chan<- JobStatus),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start begins watching for job status changes
func (jw *JobWatcher) Start() error {
	jw.wg.Add(1)
	go jw.watchJobs()
	return nil
}

// Stop stops the job watcher
func (jw *JobWatcher) Stop() {
	jw.cancel()
	jw.wg.Wait()
	
	// Close all subscriber channels
	jw.subMutex.Lock()
	defer jw.subMutex.Unlock()
	for _, channels := range jw.subscribers {
		for _, ch := range channels {
			close(ch)
		}
	}
	jw.subscribers = make(map[string][]chan<- JobStatus)
}

// GetJobStatus returns the current status of a job
func (jw *JobWatcher) GetJobStatus(identifier string) (JobStatus, bool) {
	jw.statusMutex.RLock()
	defer jw.statusMutex.RUnlock()
	
	status, exists := jw.jobStatuses[identifier]
	if !exists {
		return JobStatus{}, false
	}
	return *status, true
}

// IsJobFinished checks if a job is finished (non-blocking)
func (jw *JobWatcher) IsJobFinished(identifier string) bool {
	if status, exists := jw.GetJobStatus(identifier); exists {
		return status.Finished
	}
	
	// If not in cache, do a one-time lookup
	return jw.checkJobStatusOnce(identifier)
}

// SubscribeToJob subscribes to status changes for a specific job
func (jw *JobWatcher) SubscribeToJob(identifier string) (<-chan JobStatus, func()) {
	ch := make(chan JobStatus, 10) // Buffer to prevent blocking
	
	jw.subMutex.Lock()
	jw.subscribers[identifier] = append(jw.subscribers[identifier], ch)
	jw.subMutex.Unlock()
	
	// Send current status if available
	if status, exists := jw.GetJobStatus(identifier); exists {
		select {
		case ch <- status:
		default:
			// Channel full, skip
		}
	}
	
	// Return unsubscribe function
	unsubscribe := func() {
		jw.subMutex.Lock()
		defer jw.subMutex.Unlock()
		
		subscribers := jw.subscribers[identifier]
		for i, subscriber := range subscribers {
			if subscriber == ch {
				// Remove from slice
				jw.subscribers[identifier] = append(subscribers[:i], subscribers[i+1:]...)
				close(subscriber)
				break
			}
		}
		
		// Clean up empty slices
		if len(jw.subscribers[identifier]) == 0 {
			delete(jw.subscribers, identifier)
		}
	}
	
	return ch, unsubscribe
}

// UnsubscribeFromJob removes a subscription (call this to avoid memory leaks)
// Deprecated: Use the unsubscribe function returned by SubscribeToJob instead
func (jw *JobWatcher) UnsubscribeFromJob(identifier string, ch <-chan JobStatus) {
	// This method is kept for backward compatibility but is harder to implement
	// due to Go's channel direction restrictions. Use the unsubscribe function instead.
}

// watchJobs watches for job changes using Kubernetes watch API
func (jw *JobWatcher) watchJobs() {
	defer jw.wg.Done()
	
	for {
		select {
		case <-jw.ctx.Done():
			return
		default:
			if err := jw.startWatch(); err != nil {
				jw.logger.Errorf("Watch failed, retrying in 5 seconds: %v", err)
				select {
				case <-time.After(5 * time.Second):
					continue
				case <-jw.ctx.Done():
					return
				}
			}
		}
	}
}

// startWatch starts a watch session
func (jw *JobWatcher) startWatch() error {
	// Watch all jobs with our labels
	labelSelectors := []string{
		"etos.eiffel-community.github.io/id", // v1alpha+
		"id",                                  // v0 legacy
	}
	
	for _, labelSelector := range labelSelectors {
		watcher, err := jw.client.BatchV1().Jobs(jw.namespace).Watch(jw.ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
			Watch:         true,
		})
		if err != nil {
			return fmt.Errorf("failed to create watcher: %w", err)
		}
		
		go jw.processWatchEvents(watcher, labelSelector)
	}
	
	// Keep the watch alive until context is cancelled
	<-jw.ctx.Done()
	return nil
}

// processWatchEvents processes events from a watcher
func (jw *JobWatcher) processWatchEvents(watcher watch.Interface, labelKey string) {
	defer watcher.Stop()
	
	for {
		select {
		case <-jw.ctx.Done():
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				jw.logger.Debug("Watch channel closed, will restart")
				return
			}
			
			job, ok := event.Object.(*v1.Job)
			if !ok {
				jw.logger.Warning("Received non-Job object from watch")
				continue
			}
			
			identifier := job.Labels[labelKey]
			if identifier == "" {
				continue
			}
			
			jw.updateJobStatus(identifier, job, event.Type)
		}
	}
}

// updateJobStatus updates the status of a job and notifies subscribers
func (jw *JobWatcher) updateJobStatus(identifier string, job *v1.Job, eventType watch.EventType) {
	finished := job.Status.Succeeded > 0 || job.Status.Failed > 0
	succeeded := job.Status.Succeeded > 0
	failed := job.Status.Failed > 0
	
	status := &JobStatus{
		Identifier: identifier,
		Finished:   finished,
		Succeeded:  succeeded,
		Failed:     failed,
		LastUpdate: time.Now(),
	}
	
	// Update cache
	jw.statusMutex.Lock()
	jw.jobStatuses[identifier] = status
	jw.statusMutex.Unlock()
	
	// Notify subscribers
	jw.notifySubscribers(identifier, *status)
	
	jw.logger.WithFields(logrus.Fields{
		"identifier": identifier,
		"finished":   finished,
		"succeeded":  succeeded,
		"failed":     failed,
		"event_type": eventType,
	}).Debug("Job status updated")
}

// notifySubscribers notifies all subscribers of a job status change
func (jw *JobWatcher) notifySubscribers(identifier string, status JobStatus) {
	jw.subMutex.RLock()
	subscribers := jw.subscribers[identifier]
	jw.subMutex.RUnlock()
	
	for _, ch := range subscribers {
		select {
		case ch <- status:
		default:
			// Channel full or closed, skip
		}
	}
}

// checkJobStatusOnce performs a one-time check for jobs not in the cache
func (jw *JobWatcher) checkJobStatusOnce(identifier string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Try different labels for backward compatibility
	for _, label := range []string{"etos.eiffel-community.github.io/id", "id"} {
		jobs, err := jw.client.BatchV1().Jobs(jw.namespace).List(
			ctx,
			metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", label, identifier),
			},
		)
		if err != nil {
			jw.logger.Error(err)
			continue
		}
		
		if len(jobs.Items) == 0 {
			continue
		}
		
		job := jobs.Items[0]
		finished := job.Status.Succeeded > 0 || job.Status.Failed > 0
		
		// Update cache with this information
		jw.updateJobStatus(identifier, &job, watch.Modified)
		
		return finished
	}
	
	// Assume finished if not found
	jw.logger.Warningf("Job with id %s not found, assuming finished", identifier)
	return true
}
