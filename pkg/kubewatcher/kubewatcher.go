/*
Copyright 2016 The Fission Authors.

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

package kubewatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	fv1 "github.com/fission/fission/pkg/apis/core/v1"
	ferror "github.com/fission/fission/pkg/error"
	"github.com/fission/fission/pkg/publisher"
	"github.com/fission/fission/pkg/utils"
)

type (
	KubeWatcher struct {
		logger           *zap.Logger
		watches          map[types.UID]watchSubscription
		kubernetesClient kubernetes.Interface
		publisher        publisher.Publisher
	}

	watchSubscription struct {
		logger              *zap.Logger
		watch               fv1.KubernetesWatchTrigger
		kubeWatch           watch.Interface
		lastResourceVersion string
		stopped             *int32
		kubernetesClient    kubernetes.Interface
		publisher           publisher.Publisher
	}
)

func MakeKubeWatcher(ctx context.Context, logger *zap.Logger, kubernetesClient kubernetes.Interface, publisher publisher.Publisher) *KubeWatcher {
	kw := &KubeWatcher{
		logger:           logger.Named("kube_watcher"),
		watches:          make(map[types.UID]watchSubscription),
		kubernetesClient: kubernetesClient,
		publisher:        publisher,
	}
	return kw
}

// TODO lifted from kubernetes/pkg/kubectl/resource_printer.go.
func printKubernetesObject(obj runtime.Object, w io.Writer) error {
	switch obj := obj.(type) {
	case *runtime.Unknown:
		var buf bytes.Buffer
		err := json.Indent(&buf, obj.Raw, "", "    ")
		if err != nil {
			return err
		}
		buf.WriteRune('\n')
		_, err = buf.WriteTo(w)
		return err
	}

	data, err := json.MarshalIndent(obj, "", "    ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

func createKubernetesWatch(ctx context.Context, kubeClient kubernetes.Interface, w *fv1.KubernetesWatchTrigger, resourceVersion string) (watch.Interface, error) {
	var wi watch.Interface
	var err error
	var watchTimeoutSec int64 = 120

	// TODO populate labelselector and fieldselector
	listOptions := metav1.ListOptions{
		ResourceVersion: resourceVersion,
		TimeoutSeconds:  &watchTimeoutSec,
	}

	// TODO handle the full list of types
	switch strings.ToUpper(w.Spec.Type) {
	case "POD":
		wi, err = kubeClient.CoreV1().Pods(w.Spec.Namespace).Watch(ctx, listOptions)
	case "SERVICE":
		wi, err = kubeClient.CoreV1().Services(w.Spec.Namespace).Watch(ctx, listOptions)
	case "REPLICATIONCONTROLLER":
		wi, err = kubeClient.CoreV1().ReplicationControllers(w.Spec.Namespace).Watch(ctx, listOptions)
	case "JOB":
		wi, err = kubeClient.BatchV1().Jobs(w.Spec.Namespace).Watch(ctx, listOptions)
	default:
		err = errors.NewBadRequest(fmt.Sprintf("Error: unknown obj type '%v'", w.Spec.Type))
	}
	return wi, err
}

func (kw *KubeWatcher) addWatch(ctx context.Context, w *fv1.KubernetesWatchTrigger) error {
	kw.logger.Info("adding watch", zap.String("name", w.Name), zap.Any("function", w.Spec.FunctionReference))
	ws, err := MakeWatchSubscription(ctx, kw.logger.Named("watchsubscription"), w, kw.kubernetesClient, kw.publisher)
	if err != nil {
		return err
	}
	kw.watches[w.UID] = *ws
	return nil
}

func (kw *KubeWatcher) removeWatch(w *fv1.KubernetesWatchTrigger) error {
	kw.logger.Info("removing watch", zap.String("name", w.Name), zap.Any("function", w.Spec.FunctionReference))
	ws, ok := kw.watches[w.UID]
	if !ok {
		return ferror.MakeError(ferror.ErrorNotFound,
			fmt.Sprintf("watch doesn't exist: %v", w.ObjectMeta))
	}
	delete(kw.watches, w.UID)
	ws.stop()
	return nil
}

func MakeWatchSubscription(ctx context.Context, logger *zap.Logger, w *fv1.KubernetesWatchTrigger, kubeClient kubernetes.Interface, publisher publisher.Publisher) (*watchSubscription, error) {
	var stopped int32 = 0
	ws := &watchSubscription{
		logger:              logger.Named("watch_subscription"),
		watch:               *w,
		kubeWatch:           nil,
		stopped:             &stopped,
		kubernetesClient:    kubeClient,
		publisher:           publisher,
		lastResourceVersion: "",
	}

	err := ws.restartWatch(ctx)
	if err != nil {
		return nil, err
	}

	go ws.eventDispatchLoop(ctx)
	return ws, nil
}

func (ws *watchSubscription) restartWatch(ctx context.Context) error {
	retries := 60
	for {
		ws.logger.Info("(re)starting watch",
			zap.Any("watch", ws.watch.ObjectMeta),
			zap.String("namespace", ws.watch.Spec.Namespace),
			zap.String("type", ws.watch.Spec.Type),
			zap.String("last_resource_version", ws.lastResourceVersion))
		wi, err := createKubernetesWatch(ctx, ws.kubernetesClient, &ws.watch, ws.lastResourceVersion)
		if err != nil {
			retries--
			if retries > 0 {
				time.Sleep(500 * time.Millisecond)
				continue
			} else {
				return err
			}
		}
		ws.kubeWatch = wi
		return nil
	}
}

func getResourceVersion(obj runtime.Object) (string, error) {
	m, err := meta.Accessor(obj)
	if err != nil {
		return "", err
	}
	return m.GetResourceVersion(), nil
}

func (ws *watchSubscription) eventDispatchLoop(ctx context.Context) {
	ws.logger.Info("listening to watch", zap.String("name", ws.watch.ObjectMeta.Name))
	// check watchSubscription is stopped or not before waiting for event
	// comes from the kubeWatch.ResultChan(). This fix the edge case that
	// new kubewatch is created in the restartWatch() while the old kubewatch
	// is being used in watchSubscription.stop().
	for !ws.isStopped() {
		ev, more := <-ws.kubeWatch.ResultChan()
		if !more {
			if ws.isStopped() {
				// watch is removed by user.
				ws.logger.Warn("watch stopped", zap.String("watch_name", ws.watch.ObjectMeta.Name))
				return
			} else {
				// watch closed due to timeout, restart it.
				ws.logger.Warn("watch timed out - restarting", zap.String("watch_name", ws.watch.ObjectMeta.Name))
				err := ws.restartWatch(ctx)
				if err != nil {
					ws.logger.Panic("failed to restart watch", zap.Error(err), zap.String("watch_name", ws.watch.ObjectMeta.Name))
				}
				continue
			}
		}

		if ev.Type == watch.Error {
			e := errors.FromObject(ev.Object)
			ws.logger.Warn("watch error - retrying after one second", zap.Error(e), zap.String("watch_name", ws.watch.ObjectMeta.Name))
			// Start from the beginning to get around "too old resource version"
			ws.lastResourceVersion = ""
			time.Sleep(time.Second)
			err := ws.restartWatch(ctx)
			if err != nil {
				ws.logger.Panic("failed to restart watch", zap.Error(err), zap.String("watch_name", ws.watch.ObjectMeta.Name))
			}
			continue
		}
		rv, err := getResourceVersion(ev.Object)
		if err != nil {
			ws.logger.Error("error getting resourceVersion from object", zap.Error(err), zap.String("watch_name", ws.watch.ObjectMeta.Name))
		} else {
			ws.lastResourceVersion = rv
		}

		// Serialize the object
		var buf bytes.Buffer
		err = printKubernetesObject(ev.Object, &buf)
		if err != nil {
			ws.logger.Error("failed to serialize object", zap.Error(err), zap.String("watch_name", ws.watch.ObjectMeta.Name))
			// TODO send a POST request indicating error
		}

		// Event and object type aren't in the serialized object
		headers := map[string]string{
			"Content-Type":             "application/json",
			"X-Kubernetes-Event-Type":  string(ev.Type),
			"X-Kubernetes-Object-Type": reflect.TypeOf(ev.Object).Elem().Name(),
		}

		// TODO support other function ref types. Or perhaps delegate to router?
		if ws.watch.Spec.FunctionReference.Type != fv1.FunctionReferenceTypeFunctionName {
			ws.logger.Error("unsupported function ref type - cannot publish event",
				zap.Any("type", ws.watch.Spec.FunctionReference.Type),
				zap.String("watch_name", ws.watch.ObjectMeta.Name))
			continue
		}

		// with the addition of multi-tenancy, the users can create functions in any namespace. however,
		// the triggers can only be created in the same namespace as the function.
		// so essentially, function namespace = trigger namespace.
		url := utils.UrlForFunction(ws.watch.Spec.FunctionReference.Name, ws.watch.ObjectMeta.Namespace)
		ws.publisher.Publish(ctx, buf.String(), headers, http.MethodPost, url)
	}
}

func (ws *watchSubscription) stop() {
	atomic.StoreInt32(ws.stopped, 1)
	ws.kubeWatch.Stop()
}

func (ws *watchSubscription) isStopped() bool {
	return atomic.LoadInt32(ws.stopped) == 1
}
